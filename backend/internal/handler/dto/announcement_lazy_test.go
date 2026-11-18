package dto

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestAnnouncementTimelineOmitsBodyAndBoundsUnicodeExcerpt(t *testing.T) {
	item := &service.UserAnnouncement{Announcement: service.Announcement{ID: 1, Title: "title", Content: strings.Repeat("中文\n", 100)}}
	list := UserAnnouncementListFromService(item)
	require.Len(t, []rune(list.Excerpt), 140)
	encoded, err := json.Marshal(list)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), `"content"`)
	require.Equal(t, item.Announcement.Content, UserAnnouncementFromService(item).Content)
}
