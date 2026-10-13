package queue

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/PatchMon/PatchMon/server-source-code/internal/notifications"
)

func TestImageUpdateNotificationFields(t *testing.T) {
	var meta map[string]interface{}
	if err := json.Unmarshal([]byte(`{"host_name":"web-1","update_count":12,"images":["nginx:1.25.0","redis:7"]}`), &meta); err != nil {
		t.Fatal(err)
	}
	if got := nocImageUpdateList(meta); got != "nginx:1.25.0, redis:7 and 10 more" {
		t.Errorf("list = %q", got)
	}
	single := map[string]interface{}{"images": []interface{}{"nginx:1.25.0"}, "update_count": float64(1)}
	if got := nocImageUpdateList(single); got != "nginx:1.25.0" {
		t.Errorf("single = %q", got)
	}

	p := notifications.NotificationDeliverPayload{EventType: "container_image_update_available", Metadata: meta}
	fields := buildNOCFields(p)
	var images string
	for _, f := range fields {
		if f["name"] == "Images" {
			images, _ = f["value"].(string)
		}
	}
	if !strings.Contains(images, "redis:7") {
		t.Errorf("discord fields = %+v", fields)
	}
	if s := buildSlackNOCFields(p); !strings.Contains(s, "*Images:* nginx:1.25.0") || !strings.Contains(s, "*Host:* web-1") {
		t.Errorf("slack = %q", s)
	}
}
