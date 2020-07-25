package transform

import (
	maltego "github.com/dreadl0ck/netcap/maltego"
	"github.com/dreadl0ck/netcap/types"
	"strconv"
)

func ToHTTPStatusCodes() {
	maltego.HTTPTransform(
		nil,
		func(lt maltego.LocalTransform, trx *maltego.MaltegoTransform, http *types.HTTP, min, max uint64, profilesFile string, ipaddr string) {
			if http.SrcIP == ipaddr {
				if http.StatusCode != 0 {
					val := strconv.FormatInt(int64(http.StatusCode), 10)
					trx.AddEntity("netcap.HTTPStatusCode", val)
				}
			}
		},
		false,
	)
}