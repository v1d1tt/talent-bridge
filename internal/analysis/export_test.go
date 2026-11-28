package analysis

import "github.com/v1d1tt/talent-bridge/internal/blob"

// BlobsForTest exposes the key frame store to the tests.
func (s *Service) BlobsForTest() *blob.Store { return s.blobs }
