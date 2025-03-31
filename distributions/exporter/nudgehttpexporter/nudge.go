// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package nudgehttpexporter // import "go.opentelemetry.io/collector/exporter/nudgehttpexporter"

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	mySystem "mySystem"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/structs"

	cpu3 "github.com/shirou/gopsutil/v3/cpu"

	"go.uber.org/zap"
	"google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/proto"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer/consumererror"
	"go.opentelemetry.io/collector/exporter"
	"go.opentelemetry.io/collector/exporter/exporterhelper"
	"go.opentelemetry.io/collector/internal/statusutil"
	"go.opentelemetry.io/collector/pdata"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/plog/plognudge"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricnudge"
	"go.opentelemetry.io/collector/pdata/pprofile"
	"go.opentelemetry.io/collector/pdata/pprofile/pprofilenudge"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pdata/ptrace/ptracenudge"
)

type baseExporter struct {
	// Input configuration.
	config      *Config
	client      *http.Client
	tracesURL   string
	metricsURL  string
	logsURL     string
	profilesURL string
	logger      *zap.Logger
	settings    component.TelemetrySettings
	// Default user-agent header.
	userAgent string

	nudgeExporter *NudgeExporter
}

const (
	headerRetryAfter         = "Retry-After"
	maxHTTPResponseReadBytes = 1024 * 1024

	jsonContentType     = "application/json"
	protobufContentType = "application/octet-stream" //"application/x-protobuf"
)

const indexRawdata_MIN = -1

//var configurationNudge *ini.File = nil

// Create new exporter.
func newExporter(cfg component.Config, set exporter.Settings) (*baseExporter, error) {
	oCfg := cfg.(*Config)

	if oCfg.Endpoint != "" {
		_, err := url.Parse(oCfg.Endpoint)
		if err != nil {
			return nil, errors.New("endpoint must be a valid URL")
		}
	}

	nudgeExporter_ := newNudgeExporter(oCfg)

	/*
		var err error
		configurationNudge, err = ini.Load("configurationNudge.ini")
		if err != nil {
			log.Fatalf("Erreur lors du chargement du fichier INI: %v", err)
		}
	*/

	userAgent := fmt.Sprintf("%s/%s (%s/%s)",
		set.BuildInfo.Description, set.BuildInfo.Version, runtime.GOOS, runtime.GOARCH)

	// client construction is deferred to start
	return &baseExporter{
		config:        oCfg,
		logger:        set.Logger,
		userAgent:     userAgent,
		settings:      set.TelemetrySettings,
		nudgeExporter: nudgeExporter_,
	}, nil
}

// start actually creates the HTTP client. The client construction is deferred till This point as This
// is the only place we get hold of Extensions which are required to construct auth round tripper.
func (e *baseExporter) start(ctx context.Context, host component.Host) error {
	client, err := e.config.ClientConfig.ToClient(ctx, host, e.settings)
	if err != nil {
		return err
	}
	e.client = client
	return nil
}

/*
orig = *v1.ExportTraceServiceRequest {ResourceSpans: []*go.opentelemetry.io/collector/pdata/internal/data/protogen/trace/v1.ResourceSpans len: 1, cap: 1, [*(*"go.opentelemetry.io/collector/pdata/internal/data/protogen/trace/v1.ResourceSpans")(0xc000508c60)]} = v1.ExportTraceServiceRequest {ResourceSpans: []*go.opentelemetry.io/collector/pdata/internal/data/protogen/trace/v1.ResourceSpans len: 1, cap: 1, [*(*"go.opentelemetry.io/collector/pdata/internal/data/protogen/trace/v1.ResourceSpans")(0xc000508c60)]}

	ResourceSpans = []*v1.ResourceSpans len: 1, cap: 1, [*(*"go.opentelemetry.io/collector/pdata/internal/data/protogen/trace/v1.ResourceSpans")(0xc000508c60)]
		[0] = *(*"v1.ResourceSpans")(0xc000508c60) = v1.ResourceSpans {DeprecatedScopeSpans: []*go.opentelemetry.io/collector/pdata/internal/data/protogen/trace/v1.ScopeSpans len: 0, cap: 0, nil, Resource: go.opentelemetry.io/collector/pdata/internal/data/protogen/resource/v1.Resource {Attributes: []go.opent...
			DeprecatedScopeSpans = []*v1.ScopeSpans len: 0, cap: 0, nil
			Resource = v1.Resource {Attributes: []go.opentelemetry.io/collector/pdata/internal/data/protogen/common/v1.KeyValue len: 16, cap: 16, [(*"go.opentelemetry.io/collector/pdata/internal/data/protogen/common/v1.KeyValue")(0xc0004fa200),(*"go.opentelemetry.io/collector/pd...
			ScopeSpans = []*v1.ScopeSpans len: 4, cap: 4, [*(*"go.opentelemetry.io/collector/pdata/internal/data/protogen/trace/v1.ScopeSpans")(0xc000168000),*(*"go.opentelemetry.io/collector/pdata/internal/data/protogen/trace/v1.ScopeSpans")(0xc000168070),*(*"go.opentelemetry.io/...
				[0] = *(*"v1.ScopeSpans")(0xc000168000)
				[1] = *(*"v1.ScopeSpans")(0xc000168070)
				[2] = *(*"v1.ScopeSpans")(0xc000168620)
				[3] = *(*"v1.ScopeSpans")(0xc000168690)
			SchemaUrl = ""

state = *StateMutable (0)
*/
func (e *baseExporter) pushTraces(ctx context.Context, td ptrace.Traces) error {

	// Creation du This.Rawdata
	if e.nudgeExporter.Rawdata == nil {
		e.nudgeExporter.CreateRawdata()
	}

	// Traduction du request dans le rawdata de Nudge
	tr := ptracenudge.NewExportRequestFromTraces(td)
	e.nudgeExporter.TracesExportRequest2Rawdata(&tr)

	var err error
	var request []byte
	switch e.config.Encoding {
	case EncodingJSON:
		request, err = e.nudgeExporter.MarshalJSON()
	case EncodingProto:
		request, err = e.nudgeExporter.MarshalProto()
	default:
		err = fmt.Errorf("invalid encoding: %s", e.config.Encoding)
	}

	if err != nil {
		return consumererror.NewPermanent(err)
	}

	return e.export(ctx, e.tracesURL, request, e.tracesPartialSuccessHandler)
}

/*
Attributes =
[]v1.KeyValue len: 7, cap: 8, [(*"go.opentelemetry.io/collector/pdata/internal/data/protogen/common/v1.KeyValue")(0xc000334b00),(*"go.opentelemetry.io/collector/pdata/internal/data/protogen/common/v1.KeyValue")(0xc000334b20),(*"go.opentelemetry.io/collecto...
[0] =
v1.KeyValue {Key: "http.scheme", Value: go.opentelemetry.io/collector/pdata/internal/data/protogen/common/v1.AnyValue {Value: go.opentelemetry.io/collector/pdata/internal/data/protogen/common/v1.isAnyValue_Value(*go.opentelemetry.io/collector/pdata/interna...
[1] =
v1.KeyValue {Key: "http.method", Value: go.opentelemetry.io/collector/pdata/internal/data/protogen/common/v1.AnyValue {Value: go.opentelemetry.io/collector/pdata/internal/data/protogen/common/v1.isAnyValue_Value(*go.opentelemetry.io/collector/pdata/interna...
[2] =
v1.KeyValue {Key: "net.host.name", Value: go.opentelemetry.io/collector/pdata/internal/data/protogen/common/v1.AnyValue {Value: go.opentelemetry.io/collector/pdata/internal/data/protogen/common/v1.isAnyValue_Value(*go.opentelemetry.io/collector/pdata/inter...
[3] =
v1.KeyValue {Key: "http.flavor", Value: go.opentelemetry.io/collector/pdata/internal/data/protogen/common/v1.AnyValue {Value: go.opentelemetry.io/collector/pdata/internal/data/protogen/common/v1.isAnyValue_Value(*go.opentelemetry.io/collector/pdata/interna...
[4] =
v1.KeyValue {Key: "http.status_code", Value: go.opentelemetry.io/collector/pdata/internal/data/protogen/common/v1.AnyValue {Value: go.opentelemetry.io/collector/pdata/internal/data/protogen/common/v1.isAnyValue_Value(*go.opentelemetry.io/collector/pdata/in...
[5] =
v1.KeyValue {Key: "net.host.port", Value: go.opentelemetry.io/collector/pdata/internal/data/protogen/common/v1.AnyValue {Value: go.opentelemetry.io/collector/pdata/internal/data/protogen/common/v1.isAnyValue_Value(*go.opentelemetry.io/collector/pdata/inter...
[6] =
v1.KeyValue {Key: "http.route", Value: go.opentelemetry.io/collector/pdata/internal/data/protogen/common/v1.AnyValue {Value: go.opentelemetry.io/collector/pdata/internal/data/protogen/common/v1.isAnyValue_Value(*go.opentelemetry.io/collector/pdata/internal...
StartTimeUnixNano =
1743068162788000000 = 0x18309f4246458100
TimeUnixNano =
1743068163581000000 = 0x18309f427589b940
Count =
1 = 0x1
Sum_ =
v1.isHistogramDataPoint_Sum_(*go.opentelemetry.io/collector/pdata/internal/data/protogen/metrics/v1.HistogramDataPoint_Sum) *{Sum: 3.9392}
BucketCounts =
[]uint64 len: 16, cap: 16, [0,1,0,0,0,0,0,0,0,0,0,0,0,0,0,0]
ExplicitBounds =
[]float64 len: 15, cap: 15, [0,5,10,25,50,75,100,250,500,750,1000,2500,5000,7500,10000]
Exemplars =
[]v1.Exemplar len: 0, cap: 0, nil
*/
func (e *baseExporter) pushMetrics(ctx context.Context, md pmetric.Metrics) error {

	// Creation du This.Rawdata
	if e.nudgeExporter.Rawdata == nil {
		e.nudgeExporter.CreateRawdata()
	}

	tr := pmetricnudge.NewExportRequestFromMetrics(md)
	e.nudgeExporter.MetricsExportRequest2Rawdata(&tr)

	var err error
	var request []byte
	switch e.config.Encoding {
	case EncodingJSON:
		request, err = e.nudgeExporter.MarshalJSON()
	case EncodingProto:
		request, err = e.nudgeExporter.MarshalProto()
	default:
		err = fmt.Errorf("invalid encoding: %s", e.config.Encoding)
	}

	if err != nil {
		return consumererror.NewPermanent(err)
	}
	return e.export(ctx, e.metricsURL, request, e.metricsPartialSuccessHandler)
}

func (e *baseExporter) pushLogs(ctx context.Context, ld plog.Logs) error {

	// Creation du This.Rawdata
	if e.nudgeExporter.Rawdata == nil {
		e.nudgeExporter.CreateRawdata()
	}

	tr := plognudge.NewExportRequestFromLogs(ld)
	e.nudgeExporter.LogsExportRequest2Rawdata(&tr)

	var err error
	var request []byte
	switch e.config.Encoding {
	case EncodingJSON:
		request, err = e.nudgeExporter.MarshalJSON()
	case EncodingProto:
		request, err = e.nudgeExporter.MarshalProto()
	default:
		err = fmt.Errorf("invalid encoding: %s", e.config.Encoding)
	}

	if err != nil {
		return consumererror.NewPermanent(err)
	}

	return e.export(ctx, e.logsURL, request, e.logsPartialSuccessHandler)
}

func (e *baseExporter) pushProfiles(ctx context.Context, td pprofile.Profiles) error {

	// Creation du This.Rawdata
	if e.nudgeExporter.Rawdata == nil {
		e.nudgeExporter.CreateRawdata()
	}

	tr := pprofilenudge.NewExportRequestFromProfiles(td)
	e.nudgeExporter.ProfilesExportRequest2Rawdata(&tr)

	var err error
	var request []byte
	switch e.config.Encoding {
	case EncodingJSON:
		request, err = e.nudgeExporter.MarshalJSON()
	case EncodingProto:
		request, err = e.nudgeExporter.MarshalProto()
	default:
		err = fmt.Errorf("invalid encoding: %s", e.config.Encoding)
	}

	if err != nil {
		return consumererror.NewPermanent(err)
	}

	return e.export(ctx, e.profilesURL, request, e.profilesPartialSuccessHandler)
}

func (e *baseExporter) export(ctx context.Context, url string, request []byte, partialSuccessHandler partialSuccessHandler) error {

	// Met à nil le pointeur car flush des rawdata
	e.nudgeExporter.Rawdata = nil

	e.logger.Debug("Preparing to make HTTP request", zap.String("url", url))

	req, err := http.NewRequestWithContext(ctx, e.config.NudgeHTTPClientConfig.Method, url, bytes.NewReader(request))
	if err != nil {
		return consumererror.NewPermanent(err)
	}

	switch e.config.Encoding {
	case EncodingJSON:
		req.Header.Set("Content-Type", jsonContentType)
	case EncodingProto:
		req.Header.Set("Content-Type", protobufContentType)
	default:
		return fmt.Errorf("invalid encoding: %s", e.config.Encoding)
	}

	req.Header.Set("User-Agent", e.userAgent)

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make an HTTP request: %w", err)
	}

	defer func() {
		// Discard any remaining response body when we are done reading.
		_, _ = io.CopyN(io.Discard, resp.Body, maxHTTPResponseReadBytes)
		resp.Body.Close()
	}()

	if e.config.NudgeHTTPClientConfig.Log {
		e.logger.Info("Send http", zap.String("url", "/"+e.config.NudgeHTTPClientConfig.Method+" "+url), zap.Int64("ContentLength", req.ContentLength), zap.String("StatusCode", resp.Status))
	}

	if resp.StatusCode >= 200 && resp.StatusCode <= 299 {
		return handlePartialSuccessResponse(resp, partialSuccessHandler)
	}

	respStatus := readResponseStatus(resp)

	// Format the error message. Use the status if it is present in the response.
	var errString string
	var formattedErr error
	if respStatus != nil {
		errString = fmt.Sprintf(
			"error exporting items, request to %s responded with HTTP Status Code %d, Message=%s, Details=%v",
			url, resp.StatusCode, respStatus.Message, respStatus.Details)
	} else {
		errString = fmt.Sprintf(
			"error exporting items, request to %s responded with HTTP Status Code %d",
			url, resp.StatusCode)
	}
	formattedErr = statusutil.NewStatusFromMsgAndHTTPCode(errString, resp.StatusCode).Err()

	if !isRetryableStatusCode(resp.StatusCode) {
		return consumererror.NewPermanent(formattedErr)
	}

	// Check if the server is overwhelmed.
	// See spec https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/otlp.md#otlphttp-throttling
	isThrottleError := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable
	if isThrottleError {
		// Use Values to check if the header is present, and if present even if it is empty return ThrottleRetry.
		values := resp.Header.Values(headerRetryAfter)
		if len(values) == 0 {
			return formattedErr
		}
		// The value of Retry-After field can be either an HTTP-date or a number of
		// seconds to delay after the response is received. See https://datatracker.ietf.org/doc/html/rfc7231#section-7.1.3
		//
		// Retry-After = HTTP-date / delay-seconds
		//
		// First try to parse delay-seconds, since that is what the receiver will send.
		if seconds, err := strconv.Atoi(values[0]); err == nil {
			return exporterhelper.NewThrottleRetry(formattedErr, time.Duration(seconds)*time.Second)
		}
		if date, err := time.Parse(time.RFC1123, values[0]); err == nil {
			return exporterhelper.NewThrottleRetry(formattedErr, time.Until(date))
		}
	}
	return formattedErr
}

// Determine if the status code is retryable according to the specification.
// For more, see https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/otlp.md#failures-1
func isRetryableStatusCode(code int) bool {
	switch code {
	case http.StatusTooManyRequests:
		return true
	case http.StatusBadGateway:
		return true
	case http.StatusServiceUnavailable:
		return true
	case http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func readResponseBody(resp *http.Response) ([]byte, error) {
	if resp.ContentLength == 0 {
		return nil, nil
	}

	maxRead := resp.ContentLength

	// if maxRead == -1, the ContentLength header has not been sent, so read up to
	// the maximum permitted body size. If it is larger than the permitted body
	// size, still try to read from the body in case the value is an error. If the
	// body is larger than the maximum size, proto unmarshaling will likely fail.
	if maxRead == -1 || maxRead > maxHTTPResponseReadBytes {
		maxRead = maxHTTPResponseReadBytes
	}
	protoBytes := make([]byte, maxRead)
	n, err := io.ReadFull(resp.Body, protoBytes)

	// No bytes read and an EOF error indicates there is no body to read.
	if n == 0 && (err == nil || errors.Is(err, io.EOF)) {
		return nil, nil
	}

	// io.ReadFull will return io.ErrorUnexpectedEOF if the Content-Length header
	// wasn't set, since we will try to read past the length of the body. If This
	// is the case, the body will still have the full message in it, so we want to
	// ignore the error and parse the message.
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}

	return protoBytes[:n], nil
}

// Read the response and decode the status.Status from the body.
// Returns nil if the response is empty or cannot be decoded.
func readResponseStatus(resp *http.Response) *status.Status {
	var respStatus *status.Status
	if resp.StatusCode >= 400 && resp.StatusCode <= 599 {
		// Request failed. Read the body. OTLP spec says:
		// "Response body for all HTTP 4xx and HTTP 5xx responses MUST be a
		// Protobuf-encoded Status message that describes the problem."
		respBytes, err := readResponseBody(resp)
		if err != nil {
			return nil
		}

		// Decode it as Status struct. See https://github.com/open-telemetry/opentelemetry-specification/blob/main/specification/protocol/otlp.md#failures
		respStatus = &status.Status{}
		err = proto.Unmarshal(respBytes, respStatus)
		if err != nil {
			return nil
		}
	}

	return respStatus
}

func handlePartialSuccessResponse(resp *http.Response, partialSuccessHandler partialSuccessHandler) error {
	bodyBytes, err := readResponseBody(resp)
	if err != nil {
		return err
	}

	return partialSuccessHandler(bodyBytes, resp.Header.Get("Content-Type"))
}

type partialSuccessHandler func(bytes []byte, contentType string) error

func (e *baseExporter) tracesPartialSuccessHandler(protoBytes []byte, contentType string) error {
	if protoBytes == nil {
		return nil
	}
	exportResponse := ptracenudge.NewExportResponse()
	switch contentType {
	case protobufContentType:
		err := exportResponse.UnmarshalProto(protoBytes)
		if err != nil {
			return fmt.Errorf("error parsing protobuf response: %w", err)
		}
	case jsonContentType:
		err := exportResponse.UnmarshalJSON(protoBytes)
		if err != nil {
			return fmt.Errorf("error parsing json response: %w", err)
		}
	default:
		return nil
	}

	partialSuccess := exportResponse.PartialSuccess()
	if !(partialSuccess.ErrorMessage() == "" && partialSuccess.RejectedSpans() == 0) {
		e.logger.Warn("Partial success response",
			zap.String("message", exportResponse.PartialSuccess().ErrorMessage()),
			zap.Int64("dropped_spans", exportResponse.PartialSuccess().RejectedSpans()),
		)
	}
	return nil
}

func (e *baseExporter) metricsPartialSuccessHandler(protoBytes []byte, contentType string) error {
	if protoBytes == nil {
		return nil
	}
	exportResponse := pmetricnudge.NewExportResponse()
	switch contentType {
	case protobufContentType:
		err := exportResponse.UnmarshalProto(protoBytes)
		if err != nil {
			return fmt.Errorf("error parsing protobuf response: %w", err)
		}
	case jsonContentType:
		err := exportResponse.UnmarshalJSON(protoBytes)
		if err != nil {
			return fmt.Errorf("error parsing json response: %w", err)
		}
	default:
		return nil
	}

	partialSuccess := exportResponse.PartialSuccess()
	if !(partialSuccess.ErrorMessage() == "" && partialSuccess.RejectedDataPoints() == 0) {
		e.logger.Warn("Partial success response",
			zap.String("message", exportResponse.PartialSuccess().ErrorMessage()),
			zap.Int64("dropped_data_points", exportResponse.PartialSuccess().RejectedDataPoints()),
		)
	}
	return nil
}

func (e *baseExporter) logsPartialSuccessHandler(protoBytes []byte, contentType string) error {
	if protoBytes == nil {
		return nil
	}
	exportResponse := plognudge.NewExportResponse()
	switch contentType {
	case protobufContentType:
		err := exportResponse.UnmarshalProto(protoBytes)
		if err != nil {
			return fmt.Errorf("error parsing protobuf response: %w", err)
		}
	case jsonContentType:
		err := exportResponse.UnmarshalJSON(protoBytes)
		if err != nil {
			return fmt.Errorf("error parsing json response: %w", err)
		}
	default:
		return nil
	}

	partialSuccess := exportResponse.PartialSuccess()
	if !(partialSuccess.ErrorMessage() == "" && partialSuccess.RejectedLogRecords() == 0) {
		e.logger.Warn("Partial success response",
			zap.String("message", exportResponse.PartialSuccess().ErrorMessage()),
			zap.Int64("dropped_log_records", exportResponse.PartialSuccess().RejectedLogRecords()),
		)
	}
	return nil
}

func (e *baseExporter) profilesPartialSuccessHandler(protoBytes []byte, contentType string) error {
	if protoBytes == nil {
		return nil
	}
	exportResponse := pprofilenudge.NewExportResponse()
	switch contentType {
	case protobufContentType:
		err := exportResponse.UnmarshalProto(protoBytes)
		if err != nil {
			return fmt.Errorf("error parsing protobuf response: %w", err)
		}
	case jsonContentType:
		err := exportResponse.UnmarshalJSON(protoBytes)
		if err != nil {
			return fmt.Errorf("error parsing json response: %w", err)
		}
	default:
		return nil
	}

	partialSuccess := exportResponse.PartialSuccess()
	if !(partialSuccess.ErrorMessage() == "" && partialSuccess.RejectedProfiles() == 0) {
		e.logger.Warn("Partial success response",
			zap.String("message", exportResponse.PartialSuccess().ErrorMessage()),
			zap.Int64("dropped_samples", exportResponse.PartialSuccess().RejectedProfiles()),
		)
	}
	return nil
}

type NudgeExporter struct {
	Rawdata      *pdata.RawData
	Cmpt         int64
	AllDrives    map[string]interface{}
	Agentiduuid  string
	IndexRawdata int
	Components   []*pdata.Component
	Hostname     string

	Parent *Config
}

func newNudgeExporter(parent *Config) *NudgeExporter {
	This := &NudgeExporter{}
	This.Rawdata = nil
	This.Cmpt = 0
	This.AllDrives = map[string]interface{}{}
	This.Agentiduuid = ""
	This.Components = make([]*pdata.Component, 0)
	This.Hostname, _ = os.Hostname()
	This.Parent = parent

	This.InitDrives()

	return This
}

func (This *NudgeExporter) MarshalJSON() ([]byte, error) {
	jsonData, err := json.Marshal(This.Rawdata)
	return jsonData, err
}

func (This *NudgeExporter) MarshalProto() ([]byte, error) {
	// Buffer pour stocker les données binaires
	var buf bytes.Buffer

	// Création d'un encodeur GOB
	enc := gob.NewEncoder(&buf)

	// Encodage de la structure en binaire
	err := enc.Encode(This.Rawdata)

	return buf.Bytes(), err
}

func MapToString(attributes pcommon.Map) string {
	s := ""
	if attributes.Len() == 0 {
		return s
	}
	attributes.Range(func(k string, v pcommon.Value) bool {
		s += fmt.Sprintf("%v = %v , ", k, v.AsString())
		return true
	})

	return s[0 : len(s)-len(" , ")]
}

func EventToString(ses ptrace.SpanEventSlice) string {
	s := ""
	for i := 0; i < ses.Len(); i++ {
		se := ses.At(i)
		s += fmt.Sprintf("{ Event #%v , TimeStamp: %v , Name: %v , Attributes: {%v} } , ", i, se.Timestamp().String(), se.Name(), MapToString(se.Attributes()))
	}
	if len(s) < len(" , ") {
		return ""
	}

	return s[0 : len(s)-len(" , ")]
}

func NumberDataPointsToString(ndps pmetric.NumberDataPointSlice) string {
	x := make([]string, 0)
	for i := 0; i < ndps.Len(); i++ {
		ndp := ndps.At(i)

		s := ""
		s += fmt.Sprintf("NoRecordedValue : %v", ndp.Flags().NoRecordedValue())
		s += fmt.Sprintf("DoubleValue : %v , ", ndp.DoubleValue())
		s += fmt.Sprintf("IntValue : %v , ", ndp.IntValue())
		s += fmt.Sprintf("Timestamp : %v , ", ndp.Timestamp())
		s += fmt.Sprintf("ValueType: %v , ", ndp.ValueType())
		s += fmt.Sprintf("Attributes: {%v} , ", MapToString(ndp.Attributes()))

		x = append(x, fmt.Sprintf("{ %v }", s))
	}

	return fmt.Sprintf("[ %v ]", strings.Join(x, ","))
}

func UInt64ToString(u pcommon.UInt64Slice) string {
	s := ""
	for i := 0; i < u.Len(); i++ {
		s += fmt.Sprintf("%v,", u.At(i))
	}
	return fmt.Sprintf("[%v]", s[:len(s)-1])
}

func HistogramDataPointsToString(dps pmetric.HistogramDataPointSlice) string {
	x := make([]string, 0)
	for i := 0; i < dps.Len(); i++ {
		dp := dps.At(i)

		s := ""
		s += fmt.Sprintf("NoRecordedValue : %v", dp.Flags().NoRecordedValue())
		s += fmt.Sprintf("Count: %v , ", dp.Count())
		s += fmt.Sprintf("Max : %v , ", dp.Max())
		s += fmt.Sprintf("Min : %v , ", dp.Min())
		s += fmt.Sprintf("BucketCounts : (%v)%v , ", dp.BucketCounts().Len(), UInt64ToString(dp.BucketCounts()))
		s += fmt.Sprintf("Sum : %v , ", dp.Sum())
		s += fmt.Sprintf("Attributes: {%v} , ", MapToString(dp.Attributes()))

		x = append(x, fmt.Sprintf("{ %v }", s))
	}

	return fmt.Sprintf("[ %v ]", strings.Join(x, ","))
}

func (This *NudgeExporter) TracesExportRequest2Rawdata(tr *ptracenudge.ExportRequest) {
	traces := tr.Traces()
	rss := traces.ResourceSpans()
	for i := 0; i < rss.Len(); i++ {
		rs := rss.At(i)
		attributes := rs.Resource().Attributes()
		fmt.Println(fmt.Sprintf("ResourceSpan #%v , Attributes: {%v}", i, MapToString(attributes)))
		ss := rs.ScopeSpans()
		for j := 0; j < ss.Len(); j++ {
			s := ss.At(j)
			scope := s.Scope()
			fmt.Println(fmt.Sprintf("\tScopeSpan #%v , url: %v, InstrumentationScope{Name: %v , Version: %v , Attributes: {%v} }", j, s.SchemaUrl(), scope.Name(), scope.Version(), MapToString(scope.Attributes())))
			spans := s.Spans()
			for k := 0; k < spans.Len(); k++ {
				span := spans.At(k)
				fmt.Println(fmt.Sprintf("\t\tSpan #%v , Span{Event: {%v} , Name: %v , Flag: %v , Kind %v , ParentSpanID: %v , Status: {%v %v} , Attributes: {%v}", k, EventToString(span.Events()), span.Name(), span.Flags(), span.Kind().String(), span.ParentSpanID().String(), span.Status().Code(), span.Status().Message(), MapToString(span.Attributes())))
			}
		}
	}
}

func (This *NudgeExporter) MetricsExportRequest2Rawdata(tr *pmetricnudge.ExportRequest) {
	metrics := tr.Metrics()
	rsm := metrics.ResourceMetrics()
	for i := 0; i < rsm.Len(); i++ {
		rm := rsm.At(i)
		attributes := rm.Resource().Attributes()
		drop_attributes := rm.Resource().DroppedAttributesCount()
		fmt.Println(fmt.Sprintf("ResourceMetric #%v , Dropped: %v , Attributes: {%v}", i, drop_attributes, MapToString(attributes)))
		sms := rm.ScopeMetrics()
		for j := 0; j < sms.Len(); j++ {
			sm := sms.At(j)
			scope := sm.Scope()
			drop_attributes := scope.DroppedAttributesCount()
			fmt.Println(fmt.Sprintf("\tScopeMetric #%v , url: %v, InstrumentationScope{Name: %v , Version: %v , drop_attributes: %v , Attributes: {%v} }", j, sm.SchemaUrl(), scope.Name(), scope.Version(), drop_attributes, MapToString(scope.Attributes())))
			ms := sm.Metrics()
			for k := 0; k < ms.Len(); k++ {
				m := ms.At(k)
				eh := m.Histogram()
				//g := m.Gauge()
				eh_ := HistogramDataPointsToString(eh.DataPoints())
				//g_ := NumberDataPointsToString(g.DataPoints())
				fmt.Println(fmt.Sprintf("\t\tMetric #%v , Metric{Name: %v , Unit: %v , Type: %v , Description: %v , ExponentialHistogram: %v , Metadata: {%v}", k, m.Name(), m.Unit(), m.Type(), m.Description(), eh_, MapToString(m.Metadata())))
			}
		}
	}
}

func (This *NudgeExporter) LogsExportRequest2Rawdata(tr *plognudge.ExportRequest) {
	logs := tr.Logs()
	rss := logs.ResourceLogs()
	for i := 0; i < rss.Len(); i++ {
		rs := rss.At(i)
		attributes := rs.Resource().Attributes()
		fmt.Println(MapToString(attributes))
	}
}

func (This *NudgeExporter) ProfilesExportRequest2Rawdata(tr *pprofilenudge.ExportRequest) {
	profiles := tr.Profiles()
	rss := profiles.ResourceProfiles()
	for i := 0; i < rss.Len(); i++ {
		rs := rss.At(i)
		attributes := rs.Resource().Attributes()
		fmt.Println(MapToString(attributes))
	}
}

func (This *NudgeExporter) InitDrives() error {
	if This != nil {
		drives, err := mySystem.GetPhysicalDrives()
		This.AllDrives = map[string]interface{}{}
		if err != nil && len(drives) > 0 {
			This.AllDrives["drives"] = structs.Map(drives)
		} else {
			This.AllDrives["drives"] = nil
			return err
		}
	}

	return nil
}

func (This *NudgeExporter) CreateRawdata() {

	var service_hostname string = ""

	/*
		if len(configurationNudge.Section("NodeJS").Key("prefixeServiceName").String()) > 0 {
			service_hostname = configurationNudge.Section("NodeJS").Key("prefixeServiceName").String() + " "
		} else {
			service_hostname = ""
		}
	*/

	service_hostname += This.Hostname
	service_hostname = strings.ReplaceAll(service_hostname, " ", "_")

	if This.Rawdata == nil {
		This.Rawdata = &pdata.RawData{}

		This.Rawdata.ServerConfig = &pdata.ServerConfig{}                 // clearServerConfig()
		This.Rawdata.ThreadInfos = pdata.ThreadInfoList{}.Threads         // clearThreadinfosList()
		This.Rawdata.SystemMetrics = make([]*pdata.SystemMetricSample, 0) // clearSystemmetricsList()
		This.Rawdata.ClassDictionary = &pdata.Dictionary{}                // .clearClassdictionary()
		This.Rawdata.MethodDictionary = &pdata.Dictionary{}               // .clearMethoddictionary()
		This.Rawdata.ThreadActivity = &pdata.ThreadActivity{}             // .clearThreadactivity()

		This.Cmpt++
		This.Rawdata.Id = &This.Cmpt                        // setId(This.Cmpt++)
		This.Rawdata.AgentId = &This.Agentiduuid            // .setAgentid(This.Agentiduuid)
		This.Rawdata.Hostname = stringPtr(service_hostname) // .setHostname(service_hostname)

		// https://www.debugpointer.com/nodejs/md5-to-int-nodejs
		// 1. Calcul du hash MD5
		hash := md5.Sum([]byte(service_hostname))
		hashHex := hex.EncodeToString(hash[:])
		// 2. Extraction des 16 premiers caractères (8 octets en hexadécimal)
		hashPrefix := hashHex[:16]
		// 3. Conversion en entier non signé (BigInt base 16)
		bigIntValue := new(big.Int)
		bigIntValue.SetString(hashPrefix, 16)
		// 4. Conversion en entier signé 64 bits
		signed64 := bigIntValue.Int64()
		// 5. Conversion en int (Go n'a pas de Number comme en JS)
		hostKey := int64(signed64)

		This.Rawdata.Hostkey = numberPtr(hostKey) // .setHostkey(hostkey);
		//This.Rawdata.setSegmentdictionary();
		//This.Rawdata.setQuerydictionary();
		//This.Rawdata.setClassdictionary();
		//This.Rawdata.setMethoddictionary();
		//This.Rawdata.setUseragent("TOTO");
		//This.Rawdata.setMbeandictionary();
		//This.Rawdata.setJmsdictionary();

		//var o = This.Rawdata.toObject();

		if indexRawdata_MIN == -1 || This.IndexRawdata < indexRawdata_MIN {
			serverConfig := pdata.ServerConfig{}                        // new proto.org.nudge.buffer.ServerConfig();
			serverConfig.OsArch = stringPtr(runtime.GOARCH)             // .setOsarch(os.arch());
			serverConfig.OsName = stringPtr(runtime.GOOS)               // .setOsname(os.version());
			serverConfig.OsVersion = stringPtr(mySystem.GetOSVersion()) // .setOsversion(os.release());

			//cpus := cpu // os.cpus();
			cpus, _ := cpu3.Counts(true)
			serverConfig.AvailableProcessors = numberPtr(int32(cpus))         // .setAvailableprocessors(cpus.length);
			serverConfig.VmName = stringPtr("CollectorOTLP@" + This.Hostname) // .setVmname(`CollectorOTLP@${os.hostname}`); //  pjson.description;
			//TEMP serverConfig.VmVendor = pjson.vendor                       // .setVmvendor(pjson.vendor);
			infos, _ := cpu3.Info()
			//versions := Object.keys(process.versions).map(e => `${e}@${process.versions[e]}`).join(';');
			versions := mySystem.Map(infos, func(t cpu3.InfoStat) string {
				return t.ModelName + "|" + t.Family + "|" + strconv.FormatFloat(t.Mhz, 'f', 0, 64)
			})
			serverConfig.VmVersion = stringPtr(strings.Join(versions, ";")) // .setVmversion(versions);   // pjson.version);
			//serverConfig.setVmversion(process.version);   // pjson.version);
			//TEMP serverConfig.ServletContextName = pjson.name // .setServletcontextname(pjson.name);

			/*
			   versions: {
			       node: '17.7.1',
			       v8: '9.6.180.15-node.15',
			       uv: '1.43.0',
			       zlib: '1.2.11',
			       brotli: '1.0.9',
			       ares: '1.18.1',
			       modules: '102',
			       nghttp2: '1.47.0',
			       napi: '8',
			       llhttp: '6.0.4',
			       openssl: '3.0.1+quic',
			       cldr: '40.0',
			       icu: '70.1',
			       tz: '2021a3',
			       unicode: '14.0',
			       ngtcp2: '0.1.0-DEV',
			       nghttp3: '0.1.0-DEV'
			     },
			*/

			serverConfig.StartTime = numberPtr(time.Now().UnixMilli()) // .setStarttime(Date.now()); // PLantage lors du serialize Rawdata

			// Simule __dirname en JavaScript
			// Compatible Windows & Linux, car filepath.Join utilise le bon séparateur (\ pour Windows, / pour Linux/Mac).
			//TEMP exePath, err := os.Executable()
			//TEMP if err != nil {
			//TEMP dir := filepath.Dir(exePath) // __dirname en JS
			// Construction du chemin absolu
			//TEMP absolutePath := filepath.Join(dir, appJS)
			//TEMP serverConfig.BootClassPath = absolutePath // .setBootclasspath(path.resolve(__dirname, appJS));
			//TEMP }
			// LINUX ADAPT
			/*
			   if (Array.isArray(This.dns))
			     serverConfig.setCanonicalhostname(This.dns.reduce((a,e) => {
			       a += e.addresses.join(';');
			       return a;
			     }, "" ));
			*/
			//serverConfig.setCanonicalhostname(canonHost.name);
			//serverConfig.setHostaddress();
			serverConfig.HostName = stringPtr(service_hostname)                     // .setHostname(service_hostname);
			serverConfig.AppName = stringPtr(This.Parent.NodeJS.ApplicationName)    // .setAppname(configurationNudge.Section("NodeJS").Key("applicationName").String());
			platform := runtime.GOOS                                                // .platform();
			serverConfig.Environment = stringPtr(platform)                          // .setEnvironment(platform.toString());
			serverConfig.ServerName = stringPtr(This.Parent.NodeJS.ApplicationName) // .setServername(configurationNudge.Section("NodeJS").Key("applicationName").String());
			//TEMP serverConfig.ServerInfo = pjson.description                                                               // .setServerinfo(pjson.description);
			//TEMP serverConfig.ServerPort = process.debugPort                                                               // .setServerport(process.debugPort);
			serverConfig.InputArguments = stringPtr(strings.Join(os.Args, ";")) // .setInputarguments(process.argv.join(';'));
			//serverConfig.setDiagnosticconfig();
			//TEMP serverConfig.NudgeVersion = pjson.version // .setNudgeversion(pjson.version);

			jvmInfo := &pdata.JvmInfo{}       // new proto.org.nudge.buffer.JvmInfo();
			jvmInfo.HostName = &This.Hostname // .setHostname(service_hostname);
			kvs := make([]*pdata.KeyValue, 0) // new Array();

			/* Le 2025/03/17 14:40:00 a completer !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!
			   if (This?.resourceOTEL?.autoInstrumentations != undefined)
			   {
			     This.resourceOTEL.autoInstrumentations.forEach(otel => {
			       let kv = new proto.org.nudge.buffer.KeyValue();
			       kv.setKey(otel.nom);
			       kv.setValue(otel.version);
			       kvs.push(kv);
			     });
			   }
			*/
			for _, k := range os.Environ() {
				v := os.Getenv(k)
				kv := pdata.KeyValue{}  // new proto.org.nudge.buffer.KeyValue();
				kv.Key = stringPtr(k)   // .setKey(k);
				kv.Value = stringPtr(v) // .setValue(v);
				kvs = append(kvs, &kv)
			}
			jvmInfo.SystemProperties = kvs // .setSystempropertiesList(kvs);
			serverConfig.JvmInfo = jvmInfo // .setJvminfo(jvmInfo);

			//serverConfig.setSystemproperties(sps);

			This.Rawdata.ServerConfig = &serverConfig // .setServerconfig(serverConfig);
		}

		//var gcActivity = new proto.org.nudge.buffer.GcActivity();
		//gcActivity.setStarttime();
		//gcActivity.setEndtime();
		//gcActivity.setCollectioncount();
		//gcActivity.setCollectiontime();
		//This.Rawdata.setGcactivity(gcActivity);

		/*
		   blockSize:4096
		   busType:'SATA'
		   busVersion:'2.0'
		   description:'CT500MX500SSD1'
		   device:'\\\\.\\PhysicalDrive0'
		   devicePath:null
		   enumerator:'SCSI'
		   error:null
		   isCard:false
		   isReadOnly:false
		   isRemovable:false
		   isSCSI:true
		   isSystem:true
		   isUAS:false
		   isUSB:false
		   isVirtual:false
		   logicalBlockSize:512
		   mountpoints:(2) [{…}, {…}]
		   0:{path: 'C:\\'}
		   1:{path: 'D:\\'}
		*/
		/*

		   {
		     "http.url": "http://www.ZsdgdyahooX.com/",
		     "http.method": "GET",
		     "http.target": "/",
		     "net.peer.name": "www.ZsdgdyahooX.com",
		     "http.error_name": "Error",
		     "http.error_message": "getaddrinfo ENOTFOUND www.ZsdgdyahooX.com",
		   }

		*/
		//var systemMetric = This.Rawdata.getSystemmetricsList();
		//This.Rawdata.clearSystemmetricsList();

		classDictionary := &pdata.Dictionary{}           // new proto.org.nudge.buffer.Dictionary();
		methodDictionary := &pdata.Dictionary{}          // new proto.org.nudge.buffer.Dictionary();
		This.Rawdata.MethodDictionary = methodDictionary // .setMethoddictionary(methodDictionary);
		This.Rawdata.ClassDictionary = classDictionary   // .setClassdictionary(classDictionary);

		memoryUsage, _ := mySystem.GetMemoryWindows()

		getAlldrivesX := func() {

			systemMetricSample := &pdata.SystemMetricSample{}                                   // new proto.org.nudge.buffer.SystemMetricSample();
			dateNow := time.Now().UTC().UnixMilli()                                             // Date.now();
			systemMetricSample.Key = stringPtr("mem.free")                                      // .setKey(`mem.free`);
			systemMetricSample.Timestamp = &dateNow                                             // .setTimestamp(dateNow);
			systemMetricSample.LongValue = numberPtr(int64(memoryUsage.AvailPhys))              // .setLongvalue(os.freemem());
			This.Rawdata.SystemMetrics = append(This.Rawdata.SystemMetrics, systemMetricSample) // .addSystemmetrics(systemMetricSample);

			systemMetricSample = &pdata.SystemMetricSample{}                                    // new proto.org.nudge.buffer.SystemMetricSample();
			systemMetricSample.Key = stringPtr("mem.total")                                     // .setKey(`mem.total`);
			systemMetricSample.Timestamp = &dateNow                                             // .setTimestamp(dateNow);
			systemMetricSample.LongValue = numberPtr(int64(memoryUsage.TotalPhys))              // .setLongvalue(os.totalmem());
			This.Rawdata.SystemMetrics = append(This.Rawdata.SystemMetrics, systemMetricSample) // .addSystemmetrics(systemMetricSample);

			// Il faut au moins 5 secondes de temps pour mesurer la charge process
			//TEMP if oSU.dateNow {
			{
				/*
				   systemMetricSample = new proto.org.nudge.buffer.SystemMetricSample();
				   systemMetricSample.setKey(`proc.cpu.load`);
				   systemMetricSample.setTimestamp(oSU.dateNow);
				   systemMetricSample.setDoublevalue(oSU.cpuPercentage);
				   This.Rawdata.addSystemmetrics(systemMetricSample);
				*/

				systemMetricSample = &pdata.SystemMetricSample{} // new proto.org.nudge.buffer.SystemMetricSample();
				systemMetricSample.Key = stringPtr("cpu.load")   // .setKey(`mem.total`);
				//TEMP systemMetricSample.Timestamp = oSU.dateNow                                          // .setTimestamp(dateNow);
				//TEMP systemMetricSample.DoubleValue = oSU.cpuPercentage / 100                            // .setLongvalue(os.totalmem());
				This.Rawdata.SystemMetrics = append(This.Rawdata.SystemMetrics, systemMetricSample) // .addSystemmetrics(systemMetricSample);

				systemMetricSample = &pdata.SystemMetricSample{}
				systemMetricSample.Key = stringPtr("load")
				//TEMP systemMetricSample.Timestamp = oSU.dateNow
				//TEMP systemMetricSample.DoubleValue = oSU.cpuPercentageNode
				This.Rawdata.SystemMetrics = append(This.Rawdata.SystemMetrics, systemMetricSample) // .addSystemmetrics(systemMetricSample);
			}

			// La swap ne fonctionne pas sous NodeJS
			// swap.free => external
			// swap.total => rss
			//memoryUsage = process.memoryUsage();
			if memoryUsage.AvailPhys > 0 { //  || global.gc)
				systemMetricSample = &pdata.SystemMetricSample{}
				systemMetricSample.Key = stringPtr("swap.free")
				systemMetricSample.Timestamp = &dateNow
				systemMetricSample.LongValue = numberPtr(int64(memoryUsage.AvailPhys))
				This.Rawdata.SystemMetrics = append(This.Rawdata.SystemMetrics, systemMetricSample) // .addSystemmetrics(systemMetricSample);
			}
			if memoryUsage.TotalPhys > 0 { //  || global.gc)
				systemMetricSample = &pdata.SystemMetricSample{}
				systemMetricSample.Key = stringPtr("swap.total")
				systemMetricSample.Timestamp = &dateNow
				systemMetricSample.LongValue = numberPtr(int64(memoryUsage.TotalPhys))
				This.Rawdata.SystemMetrics = append(This.Rawdata.SystemMetrics, systemMetricSample) // .addSystemmetrics(systemMetricSample);
			}

			//TEMP if oSU.openFd != "not supported" {
			systemMetricSample = &pdata.SystemMetricSample{}
			systemMetricSample.Key = stringPtr("proc.fd.open")
			systemMetricSample.Timestamp = &dateNow
			//TEMP systemMetricSample.LongValue = oSU.openFd
			This.Rawdata.SystemMetrics = append(This.Rawdata.SystemMetrics, systemMetricSample) // .addSystemmetrics(systemMetricSample);
			//TEMP }

			//let swap = getSwapSize();
			// Linux stat.Totalswap * uint64(stat.Unit), stat.Freeswap * uint64(stat.Unit)
			// Windows
			systemMetricSample = &pdata.SystemMetricSample{}
			systemMetricSample.Key = stringPtr("disk.total#[SWAP]")
			systemMetricSample.Timestamp = &dateNow
			//systemMetricSample.setDoublevalue();
			systemMetricSample.LongValue = numberPtr(int64(memoryUsage.TotalPageFile))
			This.Rawdata.SystemMetrics = append(This.Rawdata.SystemMetrics, systemMetricSample) // .addSystemmetrics(systemMetricSample);

			systemMetricSample = &pdata.SystemMetricSample{}
			systemMetricSample.Key = stringPtr("disk.free#[SWAP]")
			systemMetricSample.Timestamp = &dateNow
			//systemMetricSample.setDoublevalue();
			systemMetricSample.LongValue = numberPtr(int64(memoryUsage.AvailPageFile))
			This.Rawdata.SystemMetrics = append(This.Rawdata.SystemMetrics, systemMetricSample) // .addSystemmetrics(systemMetricSample);

			/*
			   drives[1]: {
			     blockSize: 4096
			     busType: 'SATA'
			     busVersion: '2.0'
			     description: 'CT500MX500SSD1'
			     device: '\\\\.\\PhysicalDrive0'
			     devicePath: null
			     enumerator: 'SCSI'
			     error: null
			     isCard: false
			     isReadOnly: false
			     isRemovable: false
			     isSCSI: true
			     isSystem: true
			     isUAS: false
			     isUSB: false
			     isVirtual: false
			     logicalBlockSize: 512
			     mountpoints: (2) [{…}, {…}]
			     partitionTableType: 'gpt'
			     raw: '\\\\.\\PhysicalDrive0'
			     size: 500107862016
			   }
			*/
			if This.AllDrives != nil && This.AllDrives["drives"] != nil {
				for driveName := range This.AllDrives["drives"].(map[string]any) {
					drive := This.AllDrives["drives"].(map[string]any)[driveName].(map[string]any)

					if drive["path"].(string) != "[SWAP]" {
						key := `disk.total#${drive.device} - partition:${drive.partitionTableType} - isSystem:${drive.isSystem ? 'Oui' : 'Non'} - isUSB:${drive.isUSB ? 'Oui' : 'Non'} - isVirtual:${drive.isVirtual ? 'Oui' : 'Non'} - busType:${drive.busType} - busVersion:${drive.busVersion} - description:${drive.description} - mount(s):${drive.mountpoints.map((e) => e.path).join(',')}`
						systemMetricSample = &pdata.SystemMetricSample{}
						systemMetricSample.Key = &key
						systemMetricSample.Timestamp = numberPtr(dateNow)
						//systemMetricSample.setDoublevalue();
						//TEMP systemMetricSample.LongValue = drive.size
						This.Rawdata.SystemMetrics = append(This.Rawdata.SystemMetrics, systemMetricSample) // .addSystemmetrics(systemMetricSample);

						for i := 0; i < len(drive["mountpoints"].([]string)); i++ {
							part := drive["mountpoints"].([]any)[i].(map[string]any)
							var diskSpace any
							if part["path"].(string) != "[SWAP]" {
								//TEMP diskSpace = This.AllDrives["drives"][part.path]
							} else {
								diskSpace = mySystem.TotalFree{Total: 0 /*swap.size*/, Free: 0 /*wap.free*/}
							}

							if diskSpace != nil {
								systemMetricSample = &pdata.SystemMetricSample{}
								systemMetricSample.Key = stringPtr("disk.total#${part.path}")
								systemMetricSample.Timestamp = numberPtr(dateNow)
								//systemMetricSample.setDoublevalue();
								//TEMP systemMetricSample.LongValue = numberPtr(int64(diskSpace.size))

								This.Rawdata.SystemMetrics = append(This.Rawdata.SystemMetrics, systemMetricSample) // .addSystemmetrics(systemMetricSample);

								systemMetricSample = &pdata.SystemMetricSample{}
								systemMetricSample.Key = stringPtr("disk.free#${part.path}")
								systemMetricSample.Timestamp = numberPtr(dateNow)
								//systemMetricSample.setDoublevalue();
								//TEMP systemMetricSample.LongValue = numberPtr(int64(diskSpace.free))

								This.Rawdata.SystemMetrics = append(This.Rawdata.SystemMetrics, systemMetricSample) // .addSystemmetrics(systemMetricSample);
							}
						}
					}
				}
			}
		}
		getAlldrivesX()

		heapMemory := &pdata.HeapMemory{} // new proto.org.nudge.buffer.HeapMemory();
		heapMemory.StartTime = numberPtr(time.Now().UTC().UnixMilli())
		//TEMP heapMemory.Used = numberPtr(int64(memoryUsage.heapUsed))
		//TEMP heapMemory.Total = numberPtr(int64(memoryUsage.heapTotal))
		heapMemory.EndTime = numberPtr(time.Now().UTC().UnixMilli())
		This.Rawdata.HeapMemory = heapMemory

		threadActivity := &pdata.ThreadActivity{}                          // new proto.org.nudge.buffer.ThreadActivity();
		threadActivity.StartTime = numberPtr(time.Now().UTC().UnixMilli()) // process.startTime
		threadActivity.Count = numberPtr[int32](1)
		threadActivity.DaemonThreadCount = numberPtr[int32](0)
		threadActivity.NewThreadCount = numberPtr[int32](0)
		//TEMP cpuUsage := process.cpuUsage()
		//TEMP threadActivity.CpuTime = cpuUsage.system
		//TEMP threadActivity.UserTime = cpuUsage.user
		threadActivity.EndTime = numberPtr(time.Now().UTC().UnixMilli())
		This.Rawdata.ThreadActivity = threadActivity

		This.Rawdata.Components = This.Components

		/*
		   let argv = process.argv;
		   //getClientModule.call(This, argv[1]);
		   getClientModule = func(m) {
		     //try
		     {
		       let rFilename = require.resolve(m);
		       let dir = path.dirname(rFilename);
		       let name = path.basename(rFilename);
		       let dirNodeModules = `${dir}${path.sep}node_modules${path.sep}`;

		       let pj = path.join(dir,"package.json");
		       if (fs.existsSync(pj))
		       {
		         let component = new proto.org.nudge.buffer.Component();

		         component.setKey(m);
		         component.setFilename(m);

		         pj = require(pj);
		         const ver = pj.version;

		         //MyConsole.log(`version ${ver} : ${m.filename}`);

		         let maevenComponent = new proto.org.nudge.buffer.MavenComponent();
		         maevenComponent.setVersion(ver);
		         maevenComponent.setArtifactid(name);
		         component.addMavencomponents(maevenComponent);

		         const mfile = fs.readFileSync(m);
		         let sha1sum = crypto.createHash('sha1').update(mfile).digest("hex");
		         component.setFilesha1(sha1sum);

		         This.Rawdata.addComponents(component);

		         const deps = pj.dependencies;
		         Object.keys(deps).forEach(dep => {
		           let component = new proto.org.nudge.buffer.Component();

		           let indexjs = `${dirNodeModules}${dep}${path.sep}index.js`;
		           component.setKey(indexjs);
		           component.setFilename(indexjs);

		           let maevenComponent = new proto.org.nudge.buffer.MavenComponent();
		           maevenComponent.setVersion(deps[dep]);
		           maevenComponent.setArtifactid(indexjs);
		           component.addMavencomponents(maevenComponent);

		           if (fs.existsSync(indexjs))
		           {
		             const mfile = fs.readFileSync(indexjs);
		             let sha1sum = crypto.createHash('sha1').update(mfile).digest("hex");
		             component.setFilesha1(sha1sum);
		           }

		           This.Rawdata.addComponents(component);
		         });
		       }
		       else
		       {
		         if (global.configurationAgentNodeJS.Console.logDetail === true)
		         fmt.Println(`Erreur Components : Module not found ${pj}`);
		         //return;
		       }
		     }
		     //catch(e)
		     {
		       messages := e.message.split('\n');
		       fmt.Println(`Erreur Components : ${messages[0]}`);
		       return;
		     }

		   }

		   /* Methode sans WORKER.JS * /
		   //await Promise.all([p1]);
		*/

		This.IndexRawdata++

		if This.AllDrives != nil && This.AllDrives["drives"] != nil {
			//TEMP fmt.Println("Rawdata created : " + len(This.AllDrives["drives"]) + " lecteur(s) physique. " + len(This.AllDrives.drivesSpace) + " lecteur(s) logique(s)")
		}
		//let transactions = This.Rawdata.getTransactionsList(); // new Array();
	}
}

func stringPtr(s string) *string {
	c := strings.Clone(s)
	return &c
}
func numberPtr[T int64 | uint64 | int32 | uint32 | int | uint](s T) *T {
	c := s
	return &c
}
