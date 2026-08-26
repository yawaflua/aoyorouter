package server

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/yawaflua/aoyorouter/internal/driver/middlewares"
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/apipb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (a *AoyoRouterService) GetError(context.Context, *aoyorouter.GetErrorRequest) (*aoyorouter.GetErrorResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unimplemented")
}

// GetErrors implements [aoyorouter.AoyoRouterServiceServer].
func (a *AoyoRouterService) GetErrors(ctx context.Context, req *aoyorouter.GetErrorsRequest) (*aoyorouter.GetErrorsResponse, error) {
	requesterKey, ok := middlewares.GetApiKeyFromCtx(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}
	if requesterKey != nil && !requesterKey.IsAdmin {
		return nil, status.Error(codes.PermissionDenied, "permission denied")
	}

	dir, err := os.ReadDir("auth/logs")
	if err != nil {
		if os.IsNotExist(err) {
			return &aoyorouter.GetErrorsResponse{Status: "no errors found", Errors: []*aoyorouter.Error{}}, nil
		}
		return nil, err
	}
	var errors []*aoyorouter.Error
	for _, entry := range dir {
		if entry.IsDir() {
			continue
		}
		content, err := os.ReadFile("auth/logs/" + entry.Name())
		if err != nil {
			return nil, err
		}
		id := strings.TrimSuffix(entry.Name(), ".log")
		if errLog := parseErrorLog(id, string(content)); errLog != nil {
			errors = append(errors, errLog)
		}
	}
	return &aoyorouter.GetErrorsResponse{Errors: errors}, nil
}

func parseErrorLog(id string, content string) *aoyorouter.Error {
	sections := splitSections(content)
	info := parseKeyValue(sections["REQUEST INFO"])

	var headers []string
	for _, line := range strings.Split(sections["HEADERS"], "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			headers = append(headers, redactHeader(line))
		}
	}

	statusCode, _ := strconv.Atoi(parseKeyValue(sections["RESPONSE"])["Status"])

	return &aoyorouter.Error{
		Id:           id,
		Url:          info["URL"],
		Method:       &apipb.Method{Name: info["Method"]},
		Timestamp:    parseTimestamp(info["Timestamp"]),
		Headers:      headers,
		Body:         strings.TrimSpace(sections["REQUEST BODY"]),
		ResponseBody: extractResponseBody(sections["RESPONSE"]),
		StatusCode:   int32(statusCode),
	}
}

// sensitiveHeaders are never echoed back from an error log. These files record
// the full inbound request, so without this the Authorization header — a live
// upstream credential — was handed to any admin hitting GetErrors, and into
// whatever the frontend does with it.
var sensitiveHeaders = map[string]struct{}{
	"authorization":       {},
	"proxy-authorization": {},
	"x-api-key":           {},
	"api-key":             {},
	"cookie":              {},
	"set-cookie":          {},
	"x-cursor-checksum":   {},
}

func redactHeader(line string) string {
	idx := strings.Index(line, ":")
	if idx == -1 {
		return line
	}
	name := strings.TrimSpace(line[:idx])
	if _, ok := sensitiveHeaders[strings.ToLower(name)]; !ok {
		return line
	}
	return name + ": [REDACTED]"
}

func splitSections(content string) map[string]string {
	sections := make(map[string]string)
	var currentSection string
	var lines []string

	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "=== ") && strings.HasSuffix(trimmed, " ===") {
			if currentSection != "" {
				sections[currentSection] = strings.Join(lines, "\n")
			}
			currentSection = strings.TrimSpace(trimmed[4 : len(trimmed)-4])
			lines = nil
		} else {
			lines = append(lines, line)
		}
	}
	if currentSection != "" {
		sections[currentSection] = strings.Join(lines, "\n")
	}
	return sections
}

func parseKeyValue(content string) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.Index(line, ":")
		if idx == -1 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		value := strings.TrimSpace(line[idx+1:])
		result[key] = value
	}
	return result
}

func parseTimestamp(s string) *timestamppb.Timestamp {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return nil
	}
	return timestamppb.New(t)
}

func extractResponseBody(content string) string {
	lines := strings.Split(content, "\n")
	var bodyStarted bool
	var bodyLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !bodyStarted {
			if trimmed == "" {
				bodyStarted = true
			}
			continue
		}
		bodyLines = append(bodyLines, line)
	}
	return strings.TrimSpace(strings.Join(bodyLines, "\n"))
}
