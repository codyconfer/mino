package icons

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	_ "image/png"
)

func isPNG(b []byte) bool {
	return len(b) >= 8 && string(b[:8]) == "\x89PNG\r\n\x1a\n"
}

func isICO(b []byte) bool {
	return len(b) >= 6 && b[0] == 0 && b[1] == 0 && b[2] == 1 && b[3] == 0
}

func pngInICO(png []byte) ([]byte, error) {
	if !isPNG(png) {
		return nil, fmt.Errorf("not a png")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(png))
	if err != nil {
		return nil, err
	}
	w, h := cfg.Width, cfg.Height
	if w <= 0 || h <= 0 || w > 256 || h > 256 {
		return nil, fmt.Errorf("unsupported png size %dx%d", w, h)
	}
	entryW, entryH := byte(w), byte(h)
	if w == 256 {
		entryW = 0
	}
	if h == 256 {
		entryH = 0
	}

	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, uint16(0))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))
	buf.WriteByte(entryW)
	buf.WriteByte(entryH)
	buf.WriteByte(0)
	buf.WriteByte(0)
	_ = binary.Write(&buf, binary.LittleEndian, uint16(1))
	_ = binary.Write(&buf, binary.LittleEndian, uint16(32))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(len(png)))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(6+16))
	buf.Write(png)
	return buf.Bytes(), nil
}
