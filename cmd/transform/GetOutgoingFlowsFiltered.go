package main

import (
	"fmt"
	maltego "github.com/dreadl0ck/netcap/maltego"
	"github.com/dreadl0ck/netcap/resolvers"
	"github.com/dreadl0ck/netcap/types"
	"github.com/dustin/go-humanize"
	"os"
	"time"
)

func GetOutgoingFlowsFiltered() {

	stdOut := os.Stdout
	os.Stdout = os.Stderr
	resolvers.InitLocalDNS()
	resolvers.InitDNSWhitelist()
	os.Stdout = stdOut

	maltego.FlowTransform(
		maltego.CountOutgoingFlowBytesFiltered,
		func(lt maltego.LocalTransform, trx *maltego.MaltegoTransform, flow  *types.Flow, min, max uint64, profilesFile string, mac string, ipaddr string, top12 *[]int) {
			if flow.SrcIP == ipaddr {
				name := resolvers.LookupDNSNameLocal(flow.DstIP)
				if name != "" {
					if !resolvers.IsWhitelisted(name) {
						if isInTop12(flow.TotalSize, top12) {
							addOutFlow(trx, flow, min, max, name)
						}
					}
				} else {
					if isInTop12(flow.TotalSize, top12) {
						addOutFlow(trx, flow, min, max, flow.DstIP)
					}
				}
			}
		},
	)
}

func addOutFlow(trx *maltego.MaltegoTransform, flow *types.Flow, min, max uint64, name string) {

	ent := trx.AddEntity("netcap.Flow", flow.UID)
	ent.SetType("netcap.Flow")
	ent.SetValue(flow.UID + "\n" + name)

	di := "<h3>Flow: " + flow.SrcIP +":"+flow.SrcPort + " -> " + flow.DstIP + ":" + flow.DstPort + "</h3><p>Timestamp: " + flow.TimestampFirst + "</p><p>TimestampLast: " + flow.TimestampLast + "</p><p>Duration: " + fmt.Sprint(time.Duration(flow.Duration)) + "</p><p>TotalSize: " + humanize.Bytes(uint64(flow.TotalSize)) + "</p>"
	ent.AddDisplayInformation(di, "Netcap Info")

	//escapedName := maltego.EscapeText()
	//ent.AddProperty("label", "Label", "strict", escapedName)

	ent.SetLinkLabel(humanize.Bytes(uint64(flow.TotalSize)))
	ent.SetLinkColor("#000000")
	ent.SetLinkThickness(maltego.GetThickness(uint64(flow.TotalSize), min, max))
}