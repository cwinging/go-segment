package segment

/*
#cgo LDFLAGS: -pthread -L/root/ffmpeg_build/lib -lavformat -lavcodec -lx265 -lstdc++ -lrt -lx264 -ldl -lvpx -lpthread -lvorbisenc -lvorbis -logg -lopus -lmp3lame -lfreetype -lfdk-aac -lz -lswresample -lavutil -lm
#cgo CFLAGS: -I/root/ffmpeg_build/include -DUSE_H264BSF

#include <stdlib.h>
#include <stdio.h>
#include <string.h>
#include <math.h>
#include <sys/stat.h>
#include <fcntl.h>
#include <unistd.h>
#include <time.h>

#include "libavformat/avformat.h"
#include "libavcodec/avcodec.h"

#if LIBAVCODEC_VERSION_MICRO<100
#define CODEC_ID_MP3 AV_CODEC_ID_MP3
#define CODEC_ID_AC3 AV_CODEC_ID_AC3
#endif

#define MAX_FILENAME_LENGTH 255
#define MAX_SEGMENTS 4096

static AVStream *add_output_stream(AVFormatContext *output_format_context, AVStream *input_stream) {
	AVCodecContext *input_codec_context;
	AVCodecContext *output_codec_context;
	AVStream *output_stream;

	output_stream = avformat_new_stream(output_format_context, 0);
	if (!output_stream) {
		fprintf(stderr, "Could not allocate stream\n");
		exit(1);
	}

	input_codec_context = input_stream->codec;
	output_codec_context = output_stream->codec;

	output_codec_context->codec_id = input_codec_context->codec_id;
	output_codec_context->codec_type = input_codec_context->codec_type;
	output_codec_context->codec_tag = input_codec_context->codec_tag;
	output_codec_context->bit_rate = input_codec_context->bit_rate;
	output_codec_context->extradata = input_codec_context->extradata;
	output_codec_context->extradata_size = input_codec_context->extradata_size;

    printf("codec id=%d, timebase=%f, ticks_per_frame=%d\n", input_codec_context->codec_id,
    	av_q2d(input_codec_context->time_base), input_codec_context->ticks_per_frame);

	if (av_q2d(input_codec_context->time_base) * input_codec_context->ticks_per_frame > av_q2d(input_stream->time_base) && av_q2d(input_stream->time_base) < 1.0 / 1000) {
		output_codec_context->time_base = input_codec_context->time_base;
		output_codec_context->time_base.num *= input_codec_context->ticks_per_frame;
	} else {
		output_codec_context->time_base = input_stream->time_base;
	}
	output_stream->time_base=output_codec_context->time_base;


	switch (input_codec_context->codec_type) {
		case AVMEDIA_TYPE_AUDIO:
			output_codec_context->channel_layout = input_codec_context->channel_layout;
			output_codec_context->sample_rate = input_codec_context->sample_rate;
			output_codec_context->channels = input_codec_context->channels;
			output_codec_context->frame_size = input_codec_context->frame_size;
			if ((input_codec_context->block_align == 1 && input_codec_context->codec_id == CODEC_ID_MP3) || input_codec_context->codec_id == CODEC_ID_AC3) {
				output_codec_context->block_align = 0;
			} else {
				output_codec_context->block_align = input_codec_context->block_align;
			}
			break;
		case AVMEDIA_TYPE_VIDEO:
			output_codec_context->pix_fmt = input_codec_context->pix_fmt;
			output_codec_context->width = input_codec_context->width;
			output_codec_context->height = input_codec_context->height;
			output_codec_context->has_b_frames = input_codec_context->has_b_frames;

			break;
		default:
			break;
	}

	return output_stream;
}

int write_index_file(
		const char *index, const char *tmp_index,
		unsigned int planned_segment_duration,unsigned int numsegments, unsigned int *actual_segment_duration, unsigned int segment_number_offset,
        const char *output_prefix, const char *output_file_extension,int islast
) {
	if (numsegments<1) return 0;
	FILE *tmp_index_fp;

	unsigned int i;

	tmp_index_fp = fopen(tmp_index, "w");
	if (!tmp_index_fp) {
		fprintf(stderr, "Could not open temporary m3u8 index file (%s), no index file will be created\n", tmp_index);
		return -1;
	}


	unsigned int maxDuration = actual_segment_duration[0];

	for (i = 1; i <numsegments; i++) {
		if (actual_segment_duration[i] > maxDuration) {
		    maxDuration = actual_segment_duration[i];
	    }
	}



	fprintf(tmp_index_fp,  "#EXTM3U\n#EXT-X-MEDIA-SEQUENCE:%u\n#EXT-X-TARGETDURATION:%u\n", segment_number_offset,maxDuration);


	for (i = 0; i <numsegments; i++) {
		if (fprintf(tmp_index_fp, "#EXTINF:%u,\n%s-%u%s\n", actual_segment_duration[i], output_prefix, i+segment_number_offset, output_file_extension)<0){
			fprintf(stderr, "Failed to write to tmp m3u8 index file\n");
			return -1;
		}
	}

	if (islast) {
		if (fprintf(tmp_index_fp, "#EXT-X-ENDLIST\n")<0){
			fprintf(stderr, "Failed to write to tmp m3u8 index file\n");
			return -1;
		};
	}
	fclose(tmp_index_fp);

	return rename(tmp_index, index);
}

int init() {
	av_register_all();
	avformat_network_init();
	return 0;
}

int segment(char *input_file, char* base_dirpath, char* output_index_file, char *base_file_name, char* base_file_extension, int segmentLength, int listlen) {
	unsigned int output_index = 1;
	AVOutputFormat *ofmt = NULL;
	AVFormatContext *ic = NULL;
	AVFormatContext *oc;
	AVStream *in_video_st = NULL;
	AVStream *in_audio_st = NULL;
	AVStream *out_video_st = NULL;
	AVStream *out_audio_st = NULL;
	AVCodec *codec;
	int ret;
	int quiet = 1;
	char currentOutputFileName[MAX_FILENAME_LENGTH] = {0};
	double segment_start_time = 0.0;
	char tmp_output_index_file[MAX_FILENAME_LENGTH] = {0};

	unsigned int actual_segment_durations[MAX_SEGMENTS+1];
	double packet_time = 0;

    sprintf(tmp_output_index_file, "%s.tmp", output_index_file);


	av_register_all();
	avformat_network_init(); //just to be safe with later version and be able to handle all kind of input urls


	AVStream *in_stream = NULL;
    AVStream *out_stream = NULL;

	ret = avformat_open_input(&ic, input_file, NULL, NULL);
	if (ret != 0) {
		fprintf(stderr, "Could not open input file %s. Error %d.\n", input_file, ret);
		exit(1);
	}

	if (avformat_find_stream_info(ic, NULL) < 0) {
		fprintf(stderr, "Could not read stream information.\n");
		avformat_close_input(&ic);
		exit(1);
	}

	oc = avformat_alloc_context();
	if (!oc) {
		fprintf(stderr, "Could not allocate output context.");
		avformat_close_input(&ic);
		exit(1);
	}

	int in_video_index = -1;
	int in_audio_index = -1;
	int out_video_index = -1;
	int out_audio_index = -1;
	int i;
	int listofs = 1;

	for (i = 0; i < ic->nb_streams; i++) {
		switch (ic->streams[i]->codec->codec_type) {
			case AVMEDIA_TYPE_VIDEO:
				if (!out_video_st) {
					in_video_st=ic->streams[i];
					in_video_index = i;
					in_video_st->discard = AVDISCARD_NONE;
					out_video_st = add_output_stream(oc, in_video_st);
					out_video_index=out_video_st->index;
				}
				break;
			case AVMEDIA_TYPE_AUDIO:
				if (!out_audio_st) {
					in_audio_st=ic->streams[i];
					in_audio_index = i;
					in_audio_st->discard = AVDISCARD_NONE;
					out_audio_st = add_output_stream(oc, in_audio_st);
					out_audio_index=out_audio_st->index;
				}
				break;
			default:
				ic->streams[i]->discard = AVDISCARD_ALL;
				break;
		}
	}

	if (in_video_index == -1) {
		fprintf(stderr, "Source stream must have video component.\n");
		avformat_close_input(&ic);
		avformat_free_context(oc);
		exit(1);
	}


	if (!ofmt) ofmt = av_guess_format("mpegts", NULL, NULL);
	if (!ofmt) {
		fprintf(stderr, "Could not find MPEG-TS muxer.\n");
		exit(1);
	}

	oc->oformat = ofmt;

	if (oc->oformat->flags & AVFMT_GLOBALHEADER) oc->flags |= CODEC_FLAG_GLOBAL_HEADER;

	av_dump_format(oc, 0, base_file_name, 1);


	codec = avcodec_find_decoder(in_video_st->codec->codec_id);
	if (!codec) {
		fprintf(stderr, "Could not find video decoder, key frames will not be honored.\n");
	}
	ret = avcodec_open2(in_video_st->codec, codec, NULL);
	if (ret < 0) {
		fprintf(stderr, "Could not open video decoder, key frames will not be honored.\n");
	}


	snprintf(currentOutputFileName, MAX_FILENAME_LENGTH, "%s/%s-%u%s", base_dirpath, base_file_name, output_index, base_file_extension);

	if (avio_open(&oc->pb, currentOutputFileName,AVIO_FLAG_WRITE) < 0) {
		fprintf(stderr, "Could not open '%s'.\n", currentOutputFileName);
		exit(1);
	} else if (!quiet) {
		fprintf(stderr, "Starting segment '%s'\n", currentOutputFileName);
	}

	int r = avformat_write_header(oc, NULL);
	if (r) {
		fprintf(stderr, "Could not write mpegts header to first output file.\n");
		exit(1);
	}


    unsigned int num_segments = 0;
    int decode_done;
	int waitfirstpacket = 1;
	time_t first_frame_sec = time(NULL);
	int iskeyframe = 0;
	double vid_pts2time = (double)in_video_st->time_base.num / in_video_st->time_base.den;

#if USE_H264BSF
    AVBitStreamFilterContext* h264bsfc =  av_bitstream_filter_init("h264_mp4toannexb");
#endif
	double prev_packet_time = 0;
	do {
		AVPacket packet;

		decode_done = av_read_frame(ic, &packet);

		if (decode_done < 0) {
			break;
		}

		//get the most recent packet time
		//this time is used when the time for the final segment is printed. It may not be on the edge of
		//of a keyframe!
		if (packet.stream_index == in_video_index) {
#if USE_H264BSF
            av_bitstream_filter_filter(h264bsfc, ic->streams[in_video_index]->codec, NULL, &packet.data, &packet.size, packet.data, packet.size, 0);
#endif
			packet.stream_index = out_video_index;
			packet_time = (double) packet.pts * vid_pts2time;
			iskeyframe = packet.flags & AV_PKT_FLAG_KEY;
			if (iskeyframe && waitfirstpacket) {
				waitfirstpacket = 0;
				prev_packet_time = packet_time;
				segment_start_time = packet_time;
				first_frame_sec = time(NULL);
			}
		} else if (packet.stream_index == in_audio_index){
			packet.stream_index = out_audio_index;
			iskeyframe=0;
		} else {
			//how this got here?!
			av_free_packet(&packet);
			continue;
		}


		if (waitfirstpacket) {
			av_free_packet(&packet);
			continue;
		}

		//start looking for segment splits for videos one half second before segment duration expires. This is because the
		//segments are split on key frames so we cannot expect all segments to be split exactly equally.
		if (iskeyframe &&  ((packet_time - segment_start_time) >= (segmentLength - 0.25)) ) { //a keyframe  near or past segmentLength -> SPLIT
			//printf("key frame packet time=%f, start time=%f\n", packet_time, segment_start_time);

			avio_flush(oc->pb);
			avio_close(oc->pb);

			actual_segment_durations[num_segments] = (unsigned int) rint(prev_packet_time - segment_start_time);
			num_segments++;
			if (num_segments > listlen) { //move list to exclude last:
				snprintf(currentOutputFileName, MAX_FILENAME_LENGTH, "%s/%s-%u%s", base_dirpath, base_file_name, listofs, base_file_extension);
				unlink (currentOutputFileName);
				listofs++; num_segments--;
				memmove(actual_segment_durations,actual_segment_durations+1,num_segments*sizeof(actual_segment_durations[0]));

			}
			write_index_file(output_index_file, tmp_output_index_file, segmentLength, num_segments, actual_segment_durations, listofs,  base_file_name, base_file_extension, (num_segments >= MAX_SEGMENTS));

			if (num_segments == MAX_SEGMENTS) {
					fprintf(stderr, "Reached \"hard\" max segment number %u. If this is not live stream increase segment duration. If live segmenting set max list lenth (-m ...)\n", MAX_SEGMENTS);
					break;
			}
			output_index++;
			snprintf(currentOutputFileName, MAX_FILENAME_LENGTH, "%s/%s-%u%s", base_dirpath, base_file_name, output_index, base_file_extension);


			if (avio_open(&oc->pb, currentOutputFileName, AVIO_FLAG_WRITE) < 0) {
				fprintf(stderr, "Could not open '%s'\n", currentOutputFileName);
				break;
			} else if (!quiet) {
				fprintf(stderr, "Starting segment '%s'\n", currentOutputFileName);
			}
			fflush(stderr);
 			segment_start_time = packet_time;
			first_frame_sec=time(NULL);
		}

		if (packet.stream_index == out_video_index) {
			prev_packet_time = packet_time;
		}


		in_stream = ic->streams[packet.stream_index];
        out_stream = oc->streams[packet.stream_index];
        packet.pts = av_rescale_q_rnd(packet.pts, in_stream->time_base, out_stream->time_base, AV_ROUND_NEAR_INF | AV_ROUND_PASS_MINMAX);
        packet.dts = av_rescale_q_rnd(packet.dts, in_stream->time_base, out_stream->time_base, AV_ROUND_NEAR_INF | AV_ROUND_PASS_MINMAX);
        packet.duration = av_rescale_q(packet.duration, in_stream->time_base, out_stream->time_base);
        packet.pos = -1;

		ret = av_write_frame(oc, &packet);

		if (ret < 0) {
			fprintf(stderr, "Warning: Could not write frame of stream.\n");
		} else if (ret > 0) {
			fprintf(stderr, "End of stream requested.\n");
			av_free_packet(&packet);
			break;
		}

		av_free_packet(&packet);
	} while (!decode_done);

	if (in_video_st->codec->codec !=NULL) avcodec_close(in_video_st->codec);
	if (num_segments < MAX_SEGMENTS) {
		//make sure all packets are written and then close the last file.
		avio_flush(oc->pb);
		av_write_trailer(oc);

		for (i = 0; i < oc->nb_streams; i++) {
			av_freep(&oc->streams[i]->codec);
			av_freep(&oc->streams[i]);
		}

		avio_close(oc->pb);
		av_free(oc);

		if (num_segments>0){
			actual_segment_durations[num_segments] = (unsigned int) rint(packet_time - segment_start_time);
			if (actual_segment_durations[num_segments] == 0)   actual_segment_durations[num_segments] = 1;
			num_segments++;
			write_index_file(output_index_file, tmp_output_index_file, segmentLength, num_segments, actual_segment_durations, listofs,  base_file_name, base_file_extension, 1);
		}
	}

	avformat_close_input(&ic);

return 0;

}
*/
import "C"

import (
	"errors"
	"fmt"
	"os"
	"unsafe"
)

var (
	ErrorSegment = errors.New("segment error")
)

func Init() int {
	return int(C.init())
}

func Segment(inputFile, baseDir, indexFileName, baseFileName, baseFileExtension string, segmentLength, maxSegments int) error {
	f, err := os.Open(inputFile)
	if err != nil {
		fmt.Printf("open file=%s failed.\n", inputFile)
		return err
	}

	f.Close()

	if err := os.MkdirAll(baseDir, os.ModePerm); err != nil {
		if notexist := os.IsNotExist(err); notexist == true {
			fmt.Printf("mkdir dir=%s failed.\n", baseDir)
			return err
		}
	}

	cIndexFileName := C.CString(indexFileName)
	defer C.free(unsafe.Pointer(cIndexFileName))

	cInputFile := C.CString(inputFile)
	defer C.free(unsafe.Pointer(cInputFile))

	cBaseDir := C.CString(baseDir)
	defer C.free(unsafe.Pointer(cBaseDir))

	cBaseFileName := C.CString(baseFileName)
	defer C.free(unsafe.Pointer(cBaseFileName))

	cBaseFileExtension := C.CString(baseFileExtension)
	defer C.free(unsafe.Pointer(cBaseFileExtension))

	if ret := int(C.segment(cInputFile, cBaseDir, cIndexFileName, cBaseFileName, cBaseFileExtension, C.int(segmentLength), C.int(maxSegments))); ret != 0 {
		fmt.Printf("segment %s error.\n", inputFile)
		return ErrorSegment
	}

	return nil
}
