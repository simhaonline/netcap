package maltego

import (
	"fmt"
	"github.com/dreadl0ck/netcap/encoder"
	"strings"
	"testing"
)

// deprecated, use V2
func TestGenerateAuditRecordIcons(t *testing.T) {

	generateIcons()

	encoder.ApplyActionToCustomEncoders(func(e *encoder.CustomEncoder) {
		generateAuditRecordIcon(e.Name)
	})

	encoder.ApplyActionToLayerEncoders(func(e *encoder.LayerEncoder) {
		name := strings.ReplaceAll(e.Layer.String(), "/", "")
		generateAuditRecordIcon(name)
	})
}

func TestGenerateAuditRecordIconsV2(t *testing.T) {

	generateIcons()

	encoder.ApplyActionToCustomEncoders(func(e *encoder.CustomEncoder) {
		fmt.Println(e.Name)
		generateAuditRecordIconV2(e.Name)
	})

	encoder.ApplyActionToLayerEncoders(func(e *encoder.LayerEncoder) {
		name := strings.ReplaceAll(e.Layer.String(), "/", "")
		fmt.Println(name)
		generateAuditRecordIconV2(name)
	})
}