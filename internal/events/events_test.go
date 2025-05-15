package events_test

import (
	"strconv"
	"testing"

	"github.com/open-edge-platform/cluster-manager/v2/internal/events"
)

func TestHostCreatedHandle(t *testing.T) {
	sink := events.Sink()

	for i := 0; i < 10; i++ {
		event := events.HostCreated{
			HostId:    "test-host-id-" + strconv.Itoa(i),
			ProjectId: "test-project-id-" + strconv.Itoa(i),
		}
		sink <- event
	}

	close(sink)
}

func TestHostDeleteHandle(t *testing.T) {
	sink := events.Sink()

	for i := 0; i < 10; i++ {
		event := events.HostDeletedEvent{
			HostId:    "test-host-id-" + strconv.Itoa(i),
			ProjectId: "test-project-id-" + strconv.Itoa(i),
		}
		sink <- event
	}

	close(sink)
}

/*
func TestHostUpdateHandle(t *testing.T) {
	sink := events.Sink()

	for i := 0; i < 1; i++ {
		event := events.HostUpdate{
			HostId:    "test-host-id-" + strconv.Itoa(i),
			ProjectId: "test-project-id-" + strconv.Itoa(i),
			Labels:    map[string]string{"key": "value"},
		}
		sink <- event
	}

	close(sink)
} */
