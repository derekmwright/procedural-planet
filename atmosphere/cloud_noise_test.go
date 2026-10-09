package atmosphere

import (
	"bytes"
	"testing"
)

func TestCloudSeedRepeatability(t *testing.T) {
	first := cloudNoiseAtlas(7)
	if !bytes.Equal(first, cloudNoiseAtlas(7)) {
		t.Fatal("same planet seed changed the cloud field")
	}
	if bytes.Equal(first, cloudNoiseAtlas(8)) {
		t.Fatal("different planet seed did not change the cloud field")
	}
}
