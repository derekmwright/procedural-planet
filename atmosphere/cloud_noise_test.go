package atmosphere

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"testing"
)

func TestCloudSeedRepeatability(t *testing.T) {
	first := cloudNoiseVolume(7)
	if !bytes.Equal(first, cloudNoiseVolume(7)) {
		t.Fatal("same planet seed changed the cloud field")
	}
	if bytes.Equal(first, cloudNoiseVolume(8)) {
		t.Fatal("different planet seed did not change the cloud field")
	}
}

// These hashes were extracted from each unpadded level of the shipped seed-7
// atlas before replacing it. Native upload must preserve the world at every
// distance, including the old integer rounding of the coarse levels.
func TestCloudNoisePreservesSeededField(t *testing.T) {
	want := [...]string{
		"c26bc84f5fb75d48524490ed5af8ce0bb11a47efddbaebd26a5c7c86b086fdea",
		"18f1a02d883aa1956dac22ee2d88965a674121774563844b4b3b5cdb3d9cb077",
		"7f0769081121887650940e0c1d65e82b05fbafacfa4cbe54d4aa20e6801417ca",
		"53f8effe2882c5c5657c6c41dca58a99b27334414295d905d52b400a9a45c60e",
		"31fba517cbcd96b0c1912a4a2e6e87daf852d350db5b32877ca1ce2b5a71a4eb",
		"46c96adab8796d43735a8d4eb5f7da315d508cab60dd5e587c23ec27071f3613",
		"bc6f43778affeb38cccbd27c8d9972fd9339d9f7548c96d8b0dd8e6c64cf4478",
	}
	volume := cloudNoiseVolume(7)
	levels := append([][]byte{volume}, cloudNoiseMips(volume)...)
	if len(levels) != len(want) {
		t.Fatalf("got %d noise levels, want %d", len(levels), len(want))
	}
	for i, level := range levels {
		if got := fmt.Sprintf("%x", sha256.Sum256(level)); got != want[i] {
			t.Errorf("level %d changed the seeded field: %s", i, got)
		}
	}
}
