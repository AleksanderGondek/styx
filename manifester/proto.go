package manifester

var (
	// protocol is (mostly) json over http
	ManifestPath  = "/manifest"
	ChunkDiffPath = "/chunkdiff"
)

type (
	ManifestReq struct {
		Upstream      string
		StorePathHash string
	}
	// response is zstd of proto SignedManifest

	ChunkDiffReq struct {
		From, To    string
		AcceptAlgos []string
	}
	// response is binary diff
)