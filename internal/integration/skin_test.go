package integration

import (
	"encoding/base64"
	"fmt"
	"os"
	"testing"
)

func TestBDSSkinSizeCompatibility(t *testing.T) {
	if os.Getenv("LOBBYKEEPER_BDS_TEST") == "" {
		t.Skip("requires local BDS")
	}
	geometry, err := os.ReadFile("../../vendor-src/nemc-tan-lobby-solver/minecraft/skin_geometry.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{64, 128, 256} {
		for _, premium := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d-premium-%v", width, premium), func(t *testing.T) {
				pixels := make([]byte, width*width*4)
				for i := 0; i < len(pixels); i += 4 {
					pixels[i], pixels[i+1], pixels[i+2], pixels[i+3] = 24, 120, 240, 255
				}
				testBDSWorldExchange(t, false, false, map[string]any{
					"SkinId": fmt.Sprintf("KeeperSizeTest-%d-%v", width, premium), "SkinImageWidth": width, "SkinImageHeight": width,
					"SkinData": base64.StdEncoding.EncodeToString(pixels), "SkinGeometryData": base64.StdEncoding.EncodeToString(geometry),
					"SkinResourcePatch": base64.StdEncoding.EncodeToString([]byte(`{"geometry":{"default":"geometry.humanoid.customSlim"}}`)),
					"PremiumSkin":       premium,
				})
			})
		}
	}
}
