# go-segment

通过cgo方式调用ffmpeg api实现mp4转hls.
虽然ffmpeg原生的segment命令也可以实现切片，这里采用ffmpeg api实现，读者可以根据客户需求按照时间点或者关键帧等方式来切分，更加灵活可控。

It uses the [FFmpeg multimedia framework api](http://ffmpeg.org/) for actual file
processing, and adds an easy-to-use API for probing and converting mp4 file
to hls segment.

## Quickstart

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


## Authors and Copyright

Copyright &copy; 2015-2016. cwingging@163.com. 

