package generator

import "testing"

func TestCodecSupportCacheDoesNotMemoizeOptimisticCycle(t *testing.T) {
	a := &wireType{ID: 1, Kind: kindStruct}
	c := &wireType{ID: 2, Kind: kindStruct}
	unsupported := &wireType{ID: 3, Kind: kindMap}
	a.Struct = &structModel{Type: a, Fields: []*fieldModel{{Type: c}, {Type: unsupported}}}
	c.Struct = &structModel{Type: c, Fields: []*fieldModel{{Type: a}}}
	cache := map[codecCacheKey]bool{}
	if a.supportsCodecCached(codecModeCST, map[int]bool{}, cache) {
		t.Fatal("A reaches an unsupported map and must use the standard codec")
	}
	if c.supportsCodecCached(codecModeCST, map[int]bool{}, cache) {
		t.Fatal("C depends on A and must not retain an optimistic cycle result")
	}
}
