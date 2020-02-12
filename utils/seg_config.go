package utils

func (segment SegConfig) ArchivingDataDirectory() string {
	return segment.DataDir + "_old"
}

func (segment SegConfig) PublishingDataDirectory(sourceSegment SegConfig) string {
	return sourceSegment.DataDir
}
