package media

// AttributeName uses the published AP_ItemAttributeId numeric vocabulary.
// Unknown IDs remain visible for compatibility with future engine versions.
func AttributeName(id int) string {
	names := map[int]string{
		0: "Unknown", 1: "Type", 2: "Name", 3: "Language code", 4: "Language", 5: "Codec ID", 6: "Codec", 7: "Codec description", 8: "Chapter count", 9: "Duration", 10: "Size", 11: "Size (bytes)", 12: "Stream extension", 13: "Bitrate", 14: "Audio channels", 15: "Angle", 16: "Source filename", 17: "Sample rate", 18: "Sample size", 19: "Video dimensions", 20: "Aspect ratio", 21: "Frame rate", 22: "Stream flags", 23: "Date/time", 24: "Original title ID", 25: "Segment count", 26: "Segments", 27: "Output filename", 28: "Metadata language code", 29: "Metadata language", 30: "Tree information", 31: "Panel title", 32: "Volume name", 33: "Order weight", 34: "Output format", 35: "Output format description", 36: "Seamless information", 37: "Panel text", 38: "MKV flags", 39: "MKV flags description", 40: "Channel layout", 41: "Output codec", 42: "Output conversion", 43: "Output sample rate", 44: "Output sample size", 45: "Output channels", 46: "Output channel layout", 47: "Output channel layout ID", 48: "Output audio mix", 49: "Comment", 50: "Offset sequence ID"}
	if s, ok := names[id]; ok {
		return s
	}
	return "Attribute"
}
