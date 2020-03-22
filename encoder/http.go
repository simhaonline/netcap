/*
 * NETCAP - Traffic Analysis Framework
 * Copyright (c) 2017 Philipp Mieden <dreadl0ck [at] protonmail [dot] ch>
 *
 * THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
 * WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
 * MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
 * ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
 * WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
 * ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
 * OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
 */

package encoder

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"runtime/pprof"
	"strconv"
	"strings"
	"time"

	"github.com/dreadl0ck/gopacket"
	"github.com/dreadl0ck/gopacket/ip4defrag"
	"github.com/dreadl0ck/gopacket/layers"
	"github.com/dreadl0ck/gopacket/reassembly"
	"github.com/dreadl0ck/netcap/types"
	"github.com/evilsocket/islazy/tui"
	"github.com/golang/protobuf/proto"
	"sync"
)

var (
	defragger     = ip4defrag.NewIPv4Defragmenter()
	streamFactory = &tcpStreamFactory{doHTTP: !*nohttp}
	streamPool    = reassembly.NewStreamPool(streamFactory)
	assembler     = reassembly.NewAssembler(streamPool)

	count     = 0
	dataBytes = int64(0)
	start     = time.Now()

	errorsMap      = make(map[string]uint)
	errorsMapMutex sync.Mutex

	// HTTPActive must be set to true to decode HTTP traffic
	HTTPActive  bool
	FileStorage string
)

// TODO: move into separate file, rename to Connection to unify wording
// Stream contains both unidirectional flows for a connection
type Stream struct {
	a gopacket.Flow
	b gopacket.Flow
}

// Reverse flips source and destination
func (s Stream) Reverse() Stream {
	return Stream{
		s.a.Reverse(),
		s.b.Reverse(),
	}
}

func (s Stream) String() string {
	return s.a.String() + " : " + s.b.String()
}

// DecodeHTTP passes TCP packets to the TCP stream reassembler
// in order to decode HTTP request and responses
// CAUTION: this function must be called sequentially,
// because the stream reassembly implementation currently does not handle out of order packets
func DecodeHTTP(packet gopacket.Packet) {

	count++
	data := packet.Data()

	// lock to sync with read on destroy
	errorsMapMutex.Lock()
	dataBytes += int64(len(data))
	errorsMapMutex.Unlock()

	// defrag the IPv4 packet if required
	if !*nodefrag {
		ip4Layer := packet.Layer(layers.LayerTypeIPv4)
		if ip4Layer == nil {
			return
		}

		var (
			ip4         = ip4Layer.(*layers.IPv4)
			l           = ip4.Length
			newip4, err = defragger.DefragIPv4(ip4)
		)
		if err != nil {
			log.Fatalln("Error while de-fragmenting", err)
		} else if newip4 == nil {
			logDebug("Fragment...\n")
			return
		}
		if newip4.Length != l {
			reassemblyStats.ipdefrag++
			logDebug("Decoding re-assembled packet: %s\n", newip4.NextLayerType())
			pb, ok := packet.(gopacket.PacketBuilder)
			if !ok {
				panic("Not a PacketBuilder")
			}
			nextDecoder := newip4.NextLayerType()
			if err := nextDecoder.Decode(newip4.Payload, pb); err != nil {
				fmt.Println("failed to decode ipv4:", err)
			}
		}
	}

	tcp := packet.Layer(layers.LayerTypeTCP)
	if tcp != nil {
		tcp := tcp.(*layers.TCP)
		if *checksum {
			err := tcp.SetNetworkLayerForChecksum(packet.NetworkLayer())
			if err != nil {
				log.Fatalf("Failed to set network layer for checksum: %s\n", err)
			}
		}
		c := Context{
			CaptureInfo: packet.Metadata().CaptureInfo,
		}
		reassemblyStats.totalsz += len(tcp.Payload)

		//fmt.Println("AssembleWithContext")

		done := make(chan bool, 1)
		go func() {
			assembler.AssembleWithContext(packet.NetworkLayer().NetworkFlow(), tcp, &c)
			done <- true
		}()

		select {
		case <-done:
		case <-time.After(3 * time.Second):
			fmt.Println("assembler context timeout", packet.NetworkLayer().NetworkFlow(), packet.TransportLayer().TransportFlow())
			//fmt.Println("closed", assembler.FlushAll())
		}
		//fmt.Println("AssembleWithContext done")
	}

	// flush connections in interval
	if count%*flushevery == 0 {
		ref := packet.Metadata().CaptureInfo.Timestamp
		// flushed, closed :=
		//fmt.Println("FlushWithOptions")
		assembler.FlushWithOptions(reassembly.FlushOptions{T: ref.Add(-timeout), TC: ref.Add(-closeTimeout)})
		//fmt.Println("FlushWithOptions done")
		// fmt.Printf("Forced flush: %d flushed, %d closed (%s)\n", flushed, closed, ref, ref.Add(-timeout))
	}
}

var httpEncoder = CreateCustomEncoder(types.Type_NC_HTTP, "HTTP", func(d *CustomEncoder) error {

	// postinit:
	// set debug level
	// and ensure HTTP collection is enabled

	if *debug {
		outputLevel = 2
	} else if *verbose {
		outputLevel = 1
	} else if *quiet {
		outputLevel = -1
	}

	HTTPActive = true

	// set file storage via flag
	if *fileStorage != "" {
		FileStorage = *fileStorage
	}

	return nil
}, func(packet gopacket.Packet) proto.Message {
	// encoding func is nil, because the processing happens after TCP stream reassemblyis nil, because the processing happens after TCP stream reassembly
	return nil
}, func(e *CustomEncoder) error {

	// de-init: finishes processing
	// and prints statistics

	if !Quiet {
		errorsMapMutex.Lock()
		fmt.Fprintf(os.Stderr, "HTTPEncoder: Processed %v packets (%v bytes) in %v (errors: %v, type:%v)\n", count, dataBytes, time.Since(start), numErrors, len(errorsMap))
		errorsMapMutex.Unlock()

		// print configuration
		// print configuration as table
		tui.Table(os.Stdout, []string{"TCP Reassembly Setting", "Value"}, [][]string{
			{"FlushEvery", strconv.Itoa(*flushevery)},
			{"CloseTimeout", closeTimeout.String()},
			{"Timeout", timeout.String()},
			{"AllowMissingInit", strconv.FormatBool(*allowmissinginit)},
			{"IgnoreFsmErr", strconv.FormatBool(*ignorefsmerr)},
			{"NoOptCheck", strconv.FormatBool(*nooptcheck)},
			{"Checksum", strconv.FormatBool(*checksum)},
			{"NoDefrag", strconv.FormatBool(*nodefrag)},
			{"WriteIncomplete", strconv.FormatBool(*writeincomplete)},
		})
		fmt.Println() // add a newline
	}

	closed := assembler.FlushAll()

	if !Quiet {
		fmt.Printf("Final flush: %d closed\n", closed)
		if outputLevel >= 2 {
			streamPool.Dump()
		}
	}

	if *memprofile != "" {
		f, err := os.Create(*memprofile)
		if err != nil {
			return err
		}
		if err := pprof.WriteHeapProfile(f); err != nil {
			log.Fatal("failed to write heap profile:", err)
		}
		if err := f.Close(); err != nil {
			log.Fatal("failed to close heap profile file:", err)
		}
	}

	streamFactory.WaitGoRoutines()

	if !Quiet {
		logDebug("%s\n", assembler.Dump())
		printProgress(1, 1)
		fmt.Println("")

		rows := [][]string{}
		if !*nodefrag {
			rows = append(rows, []string{"IPdefrag", strconv.Itoa(reassemblyStats.ipdefrag)})
		}
		rows = append(rows, []string{"missed bytes", strconv.Itoa(reassemblyStats.missedBytes)})
		rows = append(rows, []string{"total packets", strconv.Itoa(reassemblyStats.pkt)})
		rows = append(rows, []string{"rejected FSM", strconv.Itoa(reassemblyStats.rejectFsm)})
		rows = append(rows, []string{"rejected Options", strconv.Itoa(reassemblyStats.rejectOpt)})
		rows = append(rows, []string{"reassembled bytes", strconv.Itoa(reassemblyStats.sz)})
		rows = append(rows, []string{"total TCP bytes", strconv.Itoa(reassemblyStats.totalsz)})
		rows = append(rows, []string{"conn rejected FSM", strconv.Itoa(reassemblyStats.rejectConnFsm)})
		rows = append(rows, []string{"reassembled chunks", strconv.Itoa(reassemblyStats.reassembled)})
		rows = append(rows, []string{"out-of-order packets", strconv.Itoa(reassemblyStats.outOfOrderPackets)})
		rows = append(rows, []string{"out-of-order bytes", strconv.Itoa(reassemblyStats.outOfOrderBytes)})
		rows = append(rows, []string{"biggest-chunk packets", strconv.Itoa(reassemblyStats.biggestChunkPackets)})
		rows = append(rows, []string{"biggest-chunk bytes", strconv.Itoa(reassemblyStats.biggestChunkBytes)})
		rows = append(rows, []string{"overlap packets", strconv.Itoa(reassemblyStats.overlapPackets)})
		rows = append(rows, []string{"overlap bytes", strconv.Itoa(reassemblyStats.overlapBytes)})

		tui.Table(os.Stdout, []string{"TCP Stat", "Value"}, rows)

		if numErrors != 0 {
			rows = [][]string{}
			for e := range errorsMap {
				rows = append(rows, []string{e, strconv.FormatUint(uint64(errorsMap[e]), 10)})
			}
			tui.Table(os.Stdout, []string{"Error Subject", "Count"}, rows)
		}

		fmt.Println("\nencountered", numErrors, "errors during processing.", "HTTP requests", requests, " responses", responses)
		fmt.Println("httpEncoder.numRequests", e.numRequests)
		fmt.Println("httpEncoder.numResponses", e.numResponses)
		fmt.Println("httpEncoder.numUnmatchedResp", e.numUnmatchedResp)
		fmt.Println("httpEncoder.numNilRequests", e.numNilRequests)
		fmt.Println("httpEncoder.numFoundRequests", e.numFoundRequests)
		fmt.Println("httpEncoder.numUnansweredRequests", e.numUnansweredRequests)
	}

	return nil
})

/*
 *	Utils
 */

// set HTTP request on types.HTTP
func setRequest(h *types.HTTP, req *http.Request) {

	// set basic info
	h.Timestamp = req.Header.Get("netcap-ts")
	h.Proto = req.Proto
	h.Method = req.Method
	h.Host = req.Host
	h.ReqContentLength = int32(req.ContentLength)
	h.ReqContentEncoding = req.Header.Get("Content-Encoding")
	h.ContentType = req.Header.Get("Content-Type")

	body, err := ioutil.ReadAll(req.Body)
	if err == nil {
		h.ContentTypeDetected = http.DetectContentType(body)

		// decompress if required
		if h.ReqContentEncoding == "gzip" {
			r, err := gzip.NewReader(bytes.NewReader(body))
			if err == nil {
				body, err = ioutil.ReadAll(r)
				if err == nil {
					h.ContentTypeDetected = http.DetectContentType(body)
				}
			}
		}
	}

	// manually replace commas, to avoid breaking them the CSV
	// use the -check flag to validate the generated CSV output and find errors quickly
	h.UserAgent = strings.Replace(req.UserAgent(), ",", "(comma)", -1)
	h.Referer = strings.Replace(req.Referer(), ",", "(comma)", -1)
	h.URL = strings.Replace(req.URL.String(), ",", "(comma)", -1)

	// retrieve ip addresses set on the request while processing
	h.SrcIP = req.Header.Get("netcap-clientip")
	h.DstIP = req.Header.Get("netcap-serverip")

	h.ReqCookies = readCookies(req.Cookies())
}

func readCookies(cookies []*http.Cookie) []*types.HTTPCookie {
	var cks = make([]*types.HTTPCookie, 0)
	for _, c := range cookies {
		if c != nil {
			cks = append(cks, &types.HTTPCookie{
				Name:     c.Name,
				Value:    c.Value,
				Path:     c.Path,
				Domain:   c.Domain,
				Expires:  uint64(c.Expires.Unix()),
				MaxAge:   int32(c.MaxAge),
				Secure:   c.Secure,
				HttpOnly: c.HttpOnly,
				SameSite: int32(c.SameSite),
			})
		}
	}
	return cks
}

func newHTTPFromResponse(res *http.Response) *types.HTTP {

	var detected string
	var contentLength = int32(res.ContentLength)

	// read body data
	body, err := ioutil.ReadAll(res.Body)
	if err == nil {

		if contentLength == -1 {
			// determine length manually
			contentLength = int32(len(body))
		}

		// decompress payload if required
		if res.Header.Get("Content-Encoding") == "gzip" {
			r, err := gzip.NewReader(bytes.NewReader(body))
			if err == nil {
				body, err = ioutil.ReadAll(r)
				if err == nil {
					detected = http.DetectContentType(body)
				}
			}
		} else {
			detected = http.DetectContentType(body)
		}
	}

	return &types.HTTP{
		ResContentLength:       contentLength,
		ResContentType:         res.Header.Get("Content-Type"),
		StatusCode:             int32(res.StatusCode),
		ServerName:             res.Header.Get("Server"),
		ResContentEncoding:     res.Header.Get("Content-Encoding"),
		ResContentTypeDetected: detected,
		ResCookies:             readCookies(res.Cookies()),
	}
}
