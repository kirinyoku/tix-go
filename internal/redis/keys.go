package redisx

import "fmt"

const ns = "tixgo:v1"

func KeyEventSummary(eventID int64) string {
	return fmt.Sprintf("%s:event:%d:summary", ns, eventID)
}

func KeyEventAvailability(eventID int64) string {
	return fmt.Sprintf("%s:event:%d:availability", ns, eventID)
}

func KeyEventSeatMap(eventID int64) string {
	return fmt.Sprintf("%s:event:%d:seatmap", ns, eventID)
}

func KeyRateLimit(scope, id string) string {
	return fmt.Sprintf("%s:rl:%s:%s", ns, scope, id)
}

func ChannelEventsChanged() string {
	return ns + ":events:changed"
}
