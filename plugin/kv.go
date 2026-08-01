package plugin

import "github.com/codyconfer/sisyphus/stream"

type KV = stream.KV

func KVOf(bc BuildContext) KV {
	if src, ok := bc.(interface{ KV() KV }); ok {
		return src.KV()
	}
	return nil
}
