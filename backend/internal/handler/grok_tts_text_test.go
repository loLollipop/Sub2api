package handler

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGrokTTSModerationAndReservationUseSpokenText(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{`{"text":"你好🙂","input":"unspoken alternative"}`, "你好🙂"},
		{`{"text":" ","input":"备用文本"}`, "备用文本"},
		{`{"text":123,"input":null,"prompt":"最终文本"}`, "最终文本"},
	} {
		require.Equal(t, tc.want, extractGrokTTSInputText([]byte(tc.body)))
		require.Equal(t, float64(len([]rune(tc.want)))/1e6, grokVoiceInflightEstimate("tts", []byte(tc.body)).AudioUnits)
	}
}
