package segment

import (
	"testing"
)

func TestSegment(t *testing.T) {
	inputFile := "1_tmp.mp4"
	baseDir := "test"
	indexFileName := "playlist.m3u8"
	baseFileName := "mongotv"
	baseFileExtension := ".ts"
	segmentLength := 5
	maxSegments := 2048
    
	// init ffmpeg avformat avcodec register
	Init()
    
	// sement mp4 file to m3u8
	if err := Segment(inputFile, baseDir, indexFileName, baseFileName, baseFileExtension, segmentLength, maxSegments); err != nil {
		t.Errorf("segment file=%s faild.\n", inputFile)
	}
}
