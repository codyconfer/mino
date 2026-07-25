package icons

import "runtime"

func prepareAsset(mime string, b []byte) (string, []byte) {
	if len(b) == 0 {
		return mime, b
	}
	if runtime.GOOS != "windows" {
		return mime, b
	}
	if isICO(b) {
		return "image/x-icon", b
	}
	if isPNG(b) {
		ico, err := pngInICO(b)
		if err != nil {
			return mime, b
		}
		return "image/x-icon", ico
	}
	return mime, b
}

func overrideExts() []string {
	if runtime.GOOS == "windows" {
		return []string{".ico", ".png"}
	}
	return []string{".png", ".ico"}
}
