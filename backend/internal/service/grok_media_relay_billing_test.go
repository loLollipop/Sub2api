package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGrokVideoRelaySuccessShapes(t *testing.T) {
	for _, status := range []string{"done", "succeeded", "success", "completed", "finished", " COMPLETED "} {
		for _, asset := range []string{`"video":{"url":"https://example.test/v.mp4"}`, `"data":[{}, {"url":"https://example.test/v.mp4"}]`, `"data":{"url":"https://example.test/v.mp4"}`, `"url":"https://example.test/v.mp4"`, `"output":[{}, {"url":"https://example.test/v.mp4"}]`, `"output":{"url":"https://example.test/v.mp4"}`} {
			body := []byte(`{"status":"` + status + `",` + asset + `,"seconds":6}`)
			require.True(t, IsGrokVideoStatusBillable(body), string(body))
			result := ExtractGrokVideoBillingFromStatusBody(body, nil, "task")
			require.NotNil(t, result)
			require.Equal(t, 1, result.VideoCount)
			require.Equal(t, 6, result.VideoDurationSeconds)
		}
	}
	for _, body := range []string{
		`{"status":"failed","url":"https://example.test/v.mp4"}`, `{"status":"expired","url":"https://example.test/v.mp4"}`, `{"status":"unknown","url":"https://example.test/v.mp4"}`,
		`{"status":"completed"}`, `{"status":"completed","url":" "}`, `{"status":"completed","video":{"url":1}}`,
		`{"status":"completed","data":[{"url":true},{"url":{}},{"url":[]},{"url":null}]}`, `{"status":"completed","output":{"url":42}}`,
	} {
		require.False(t, IsGrokVideoStatusBillable([]byte(body)), body)
		require.Nil(t, ExtractGrokVideoBillingFromStatusBody([]byte(body), nil, "task"))
	}
}

func TestGrokVideoRelayDurationValidationAndPrecedence(t *testing.T) {
	pending := &GrokVideoPendingBilling{VideoDurationSeconds: 8, VideoResolution: "720p"}
	for _, tc := range []struct {
		fields string
		want   int
	}{
		{`"video":{"duration":12},"seconds":6,"duration":4`, 12},
		{`"seconds":6,"duration":4`, 6}, {`"duration":4`, 4},
		{`"seconds":-1,"duration":5`, 5}, {`"seconds":1e999,"duration":7`, 7},
		{`"seconds":0,"duration":-1`, 8}, {`"seconds":"6","duration":true`, 8},
		{`"seconds":0.5`, 8},
	} {
		result := ExtractGrokVideoBillingFromStatusBody([]byte(`{"status":"completed","url":"https://example.test/v.mp4",`+tc.fields+`}`), pending, "task")
		require.NotNil(t, result)
		require.Equal(t, tc.want, result.VideoDurationSeconds, tc.fields)
		require.Equal(t, "720p", result.VideoResolution)
	}
}
