package targets

import "github.com/sw33tLie/bbscope/v2/pkg/storage"

func mkEntries(targets ...struct{ t string; inScope bool }) []storage.Entry {
	var result []storage.Entry
	for _, tt := range targets {
		result = append(result, storage.Entry{
			ProgramURL:       "https://hackerone.com/test",
			TargetNormalized: tt.t,
			InScope:          tt.inScope,
		})
	}
	return result
}

func e(target string, inScope bool) struct{ t string; inScope bool } {
	return struct{ t string; inScope bool }{target, inScope}
}
