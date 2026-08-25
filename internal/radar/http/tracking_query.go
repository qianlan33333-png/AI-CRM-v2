package http

import (
	"net/url"
	"strconv"
	"time"

	radarport "github.com/qianlan33333-png/AI-CRM-v2/internal/radar/port"
)

func parseEventQuery(linkID radarport.LinkID, query url.Values) (radarport.EventListInput, error) {
	for key, values := range query {
		if len(values) != 1 {
			return radarport.EventListInput{}, radarport.ErrInvalidArgument
		}
		switch key {
		case "limit", "offset", "stage", "start_at", "end_at":
		default:
			return radarport.EventListInput{}, radarport.ErrInvalidArgument
		}
	}
	result := radarport.EventListInput{LinkID: linkID, Limit: radarport.DefaultEventLimit}
	if value := query.Get("limit"); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			return radarport.EventListInput{}, err
		}
		result.Limit = int32(parsed)
	}
	if value := query.Get("offset"); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			return radarport.EventListInput{}, err
		}
		result.Offset = int32(parsed)
	}
	if value := query.Get("stage"); value != "" {
		stage := radarport.EventStage(value)
		result.Stage = &stage
	}
	if value := query.Get("start_at"); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return radarport.EventListInput{}, err
		}
		result.Start = &parsed
	}
	if value := query.Get("end_at"); value != "" {
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return radarport.EventListInput{}, err
		}
		result.End = &parsed
	}
	return result, nil
}
