package dto

type DockerImageStats struct {
	TotalSize        int64 `json:"total_size"`
	ReclaimableSize  int64 `json:"reclaimable_size"`
	ImageCount       int   `json:"image_count"`
	ReclaimableCount int   `json:"reclaimable_count"`
}

type DockerImagePruneResult struct {
	SpaceReclaimed uint64 `json:"space_reclaimed"`
}

type BackupStats struct {
	TotalSize        int64 `json:"total_size"`
	ReclaimableSize  int64 `json:"reclaimable_size"`
	EntryCount       int   `json:"entry_count"`
	ReclaimableCount int   `json:"reclaimable_count"`
}

type BackupPruneResult struct {
	SpaceReclaimed int64 `json:"space_reclaimed"`
	RemovedCount   int   `json:"removed_count"`
}
