package dto

type DockerBuildCacheStats struct {
	TotalSize       int64 `json:"total_size"`
	ReclaimableSize int64 `json:"reclaimable_size"`
	RecordCount     int   `json:"record_count"`
}

type DockerBuildCachePruneResult struct {
	SpaceReclaimed uint64 `json:"space_reclaimed"`
}
