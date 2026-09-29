package wav

import (
	"bytes"
	"encoding/binary"
	"errors"
)

var errInvalidWAV = errors.New("invalid WAV format")

func PCMToWAV(pcm []byte, sampleRate, channels, bitsPerSample int) []byte {
	var buf bytes.Buffer
	dataSize := uint32(len(pcm))
	fileSize := dataSize + 36

	buf.Write([]byte("RIFF"))
	_ = binary.Write(&buf, binary.LittleEndian, fileSize)
	buf.Write([]byte("WAVE"))
	buf.Write([]byte("fmt "))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(channels))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(sampleRate*channels*bitsPerSample/8))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(channels*bitsPerSample/8))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(bitsPerSample))
	buf.Write([]byte("data"))
	_ = binary.Write(&buf, binary.LittleEndian, dataSize)
	buf.Write(pcm)

	return buf.Bytes()
}

func WAVToPCM(wav []byte) (pcm []byte, sampleRate, channels, bitsPerSample int, err error) {
	if len(wav) < 44 {
		return nil, 0, 0, 0, errInvalidWAV
	}
	if string(wav[0:4]) != "RIFF" {
		return nil, 0, 0, 0, errInvalidWAV
	}
	if string(wav[8:12]) != "WAVE" {
		return nil, 0, 0, 0, errInvalidWAV
	}
	if string(wav[12:16]) != "fmt " {
		return nil, 0, 0, 0, errInvalidWAV
	}

	fmtChunkSize := binary.LittleEndian.Uint32(wav[16:20])
	if fmtChunkSize != 16 {
		return nil, 0, 0, 0, errInvalidWAV
	}
	format := binary.LittleEndian.Uint16(wav[20:22])
	if format != 1 {
		return nil, 0, 0, 0, errInvalidWAV
	}

	channels = int(binary.LittleEndian.Uint16(wav[22:24]))
	sampleRate = int(binary.LittleEndian.Uint32(wav[24:28]))
	bitsPerSample = int(binary.LittleEndian.Uint16(wav[34:36]))

	return wav[44:], sampleRate, channels, bitsPerSample, nil
}

func WAVToPCM16(wavData []byte) []int16 {
	pcmBytes, _, _, _, _ := WAVToPCM(wavData)
	n := len(pcmBytes) / 2
	pcm := make([]int16, n)
	for i := 0; i < n; i++ {
		pcm[i] = int16(pcmBytes[i*2]) | int16(pcmBytes[i*2+1])<<8
	}
	return pcm
}
