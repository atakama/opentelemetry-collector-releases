// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

//go:build windows

package nudgehttpexporter // import "go.opentelemetry.io/collector/exporter/nudgehttpexporter"

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"math/big"
	mySystem "mySystem"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	extensionlinux "extensionlinux"

	"gonum.org/v1/gonum/stat"

	"github.com/fatih/structs"
	"github.com/google/uuid"
	gateway "github.com/jackpal/gateway"
	"github.com/mssola/user_agent"

	"github.com/shirou/gopsutil/v3/cpu"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
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

const (
	LAYER_TYPE_HTTP = "HTTP"
	LAYER_TYPE_SQL  = "SQL"

	// Marquée pour ne pas attendre la fin puis être envoyé dans le rawdata
	Urlid_NOWAITANDSEND = 0
	// Marquée pour attendre la fin puis être envoyé dans le rawdata si pas de parentSpanId
	Urlid_WAITANDSEND = 1
	// Marquée comme non envoyée et supprimée de la liste des transactions
	Urlid_NOWAITANDNOSEND = 2

	// 1 => erreur HTTP , 2 => erreur SQL
	Error_HTTP = 1
	Error_SQL  = 2

	Nudge_application_id = "nudge_application_id"
)

type Telemetry struct {
	Name        string
	Version     string
	Language    string
	ServiceName string
}

func (This *Telemetry) ToString() string {
	return fmt.Sprintf("Telemetry Sdk : %v %v , %v , %v",
		This.Name,
		This.Version,
		This.Language,
		This.ServiceName)
}

type Host struct {
	Name string
	Id   string
	Arch string
}

func (This *Host) ToString() string {
	return fmt.Sprintf("Host Sdk : %v, %v, %v",
		This.Name,
		This.Id,
		This.Arch,
	)
}

type Resource struct {
	Telemetry Telemetry
	Host      Host
}

type baseExporter struct {
	// Input configuration.
	config      *Config
	client      *http.Client
	tracesURL   string // URL for traces
	metricsURL  string // URL for metrics
	logsURL     string // URL for logs
	profilesURL string // URL for profiles
	logger      *zap.Logger
	settings    component.TelemetrySettings
	// Default user-agent header.
	userAgent string

	nudgeExporter *NudgeExporter
}

type ErrorX struct {
	*pdata.Error
	Stack           string
	ConstructorName string
}

func newError() *ErrorX {
	r := &ErrorX{
		Error: &pdata.Error{},
	}
	return r
}
func (e *ErrorX) ToError() *pdata.Error {
	p := &pdata.Error{}
	p.Reset()
	p.ServerId = e.ServerId
	p.Code = e.Code
	p.StartTime = e.StartTime
	p.Message = e.Message
	p.JvmStacktrace = e.JvmStacktrace
	p.Stacktrace = e.Stacktrace
	return p
}

/*
En Go, tu ne peux pas mélanger :
les initialisations positionnelles (s)
avec des initialisations nommées (Error: nil)
C’est soit tout positionnel (rare, très fragile), soit tout nommé (fortement recommandé ✅).
*/
type SpanX struct {
	ptrace.Span
	ErrorX  *ErrorX
	ErrorsX []*ErrorX
}

func newSpanX(s ptrace.Span) SpanX {
	return SpanX{
		Span:    s,
		ErrorX:  nil,
		ErrorsX: []*ErrorX{},
	}
}

const (
	headerRetryAfter         = "Retry-After"
	maxHTTPResponseReadBytes = 1024 * 1024

	jsonContentType     = "application/json"
	protobufContentType = "application/x-protobuf"
)

var (
	Headers = map[string]string{
		"content-length-uncompressed": "Content-Length",
		"content-length":              "Content-Length",
	}
)

const indexRawdata_MIN = -1

var filename_N int = 0
var serverHttpOnce sync.Once
var metricsSystemOnce sync.Once

type Mesure[T any] struct {
	StartTime time.Time `json:"startTime"`
	EndTime   time.Time `json:"endTime"`
	Values    []T       `json:"values"`
	Value     T         `json:"value"`
}

type Metrics struct {
	// CPU usage in percentage
	CPUUsage Mesure[float64] `json:"cpuUsage"`
	// Memory usage in bytes
	//MemoryUsage Mesure[int64] `json:"memoryUsage"`
	// Disk usage in bytes
	//DiskUsage Mesure[int64] `json:"diskUsage"`
	// Network usage in bytes
	//NetworkUsage Mesure[int64] `json:"networkUsage"`
}

// Metriques systeme interne sans passer pas OTLP
var metricsInterne Metrics = Metrics{}

// var configurationNudge *ini.File = nil
type Float64Array []float64

func (fa Float64Array) MarshalLogArray(enc zapcore.ArrayEncoder) error {
	for _, f := range fa {
		f = float64(int(f*1)) / 1.0 // arrondi à 1 chiffres après la virgule
		enc.AppendFloat64(f)
	}
	return nil
}

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

	userAgent := fmt.Sprintf("%s/%s (%s/%s)",
		set.BuildInfo.Description, set.BuildInfo.Version, runtime.GOOS, runtime.GOARCH)

	// client construction is deferred to start
	be := &baseExporter{
		config:        oCfg,
		logger:        set.Logger,
		userAgent:     userAgent,
		settings:      set.TelemetrySettings,
		nudgeExporter: nudgeExporter_,
	}

	// Lancement du thread unique pour le CPU
	metricsSystemOnce.Do(func() {
		go func(e *baseExporter) {
			for {
				metricsInterne.CPUUsage.StartTime = time.Now().UTC()
				metricsInterne.CPUUsage.Values, _ = cpu.Percent(time.Duration(e.nudgeExporter.Parent.RefreshLoadCPU)*time.Second, true)
				metricsInterne.CPUUsage.EndTime = time.Now().UTC()
				metricsInterne.CPUUsage.Value = stat.Mean(metricsInterne.CPUUsage.Values, nil)
				if e.nudgeExporter.Parent.Console.Verbosity.String() == "Detailed" {
					e.logger.Info("CPU", zap.Int("total", len(metricsInterne.CPUUsage.Values)), zap.Array("pourcentages", Float64Array(metricsInterne.CPUUsage.Values)))
				}

				time.Sleep(time.Second)
			}
		}(be)
	})

	return be, nil
}

// start actually creates the HTTP client. The client construction is deferred till This point as This
// is the only place we get hold of Extensions which are required to construct auth round tripper.
func (e *baseExporter) start(ctx context.Context, host component.Host) error {
	//client, err := e.config.ClientConfig.ToClient(ctx, host, e.settings)
	client := &http.Client{}
	/*
		if err != nil {
			return err
		}
	*/
	e.client = client

	port := e.config.DebugPort
	path := e.config.DebugPath

	serverHttpOnce.Do(func() {
		go func() {
			if port > 0 {
				// Créer un NOUVEAU serveur HTTP pour le debug
				mux := http.NewServeMux()

				// Charger le template depuis le fichier NudgeExporter.html
				/*tmpl, err := template.ParseFiles("WWW/NudgeExporter.html")
				if err != nil {
					log.Fatalf("Erreur de parsing template : %v", err)
				}
				*/

				// Sert les fichiers CSS/JS/images depuis le dossier ./files
				// Le chemin d'accès doit être relatif au répertoire de travail actuel : otelcol.exe
				dir := http.Dir("./WWW/files")
				mux.Handle("/files/", http.StripPrefix("/files/", http.FileServer(dir)))

				mux.HandleFunc(path+"/index.html", func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "text/html; charset=utf-8")

					mapNudgeHTTPClientConfig_FieldsComments := extensionlinux.GetFieldsComments(e.config.NudgeHTTPClientConfig)

					data := map[string]interface{}{
						"Title":              "NudgeHttpExporter actif © Atakama-Technologies 2018-2025",
						"TimeNow":            time.Now().Format(time.RFC3339),
						"CPU":                metricsInterne.CPUUsage.Value, // donnée CPU
						"Nudgehttp":          *e.config,
						"Nudgehttp_Comments": mapNudgeHTTPClientConfig_FieldsComments,
					}

					tmpl, err := template.ParseFiles("WWW/NudgeExporter.html")
					if err != nil {
						log.Fatalf("Erreur de parsing template : %v", err)
					}

					if err := tmpl.Execute(w, data); err != nil {
						http.Error(w, "Erreur serveur", http.StatusInternalServerError)
						log.Println("Erreur template:", err)
					}
				})

				addr := fmt.Sprintf("localhost:%d", port)
				if err := http.ListenAndServe(addr, mux); err != nil {
					e.logger.Error("Lancement du Serveur de page personnelles 'NudgeHttpExporter' a échoué", zap.Error(err))
				} else {
					e.logger.Info("Le Serveur de page personnelles 'NudgeHttpExporter' a démarré", zap.String("addr", addr))
					e.logger.Info("Page personnelles 'NudgeHttpExporter' disponible sur", zap.String("url", fmt.Sprintf("http://localhost:%d"+path, port)))
				}
			} else {
				e.logger.Info("Le Serveur de page personnelles 'NudgeHttpExporter' n'a pas démarré car le port est nul ou négatif")
			}
		}()
	})

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

	application_ids := extensionlinux.Reduce(e.nudgeExporter.Transactions, []string{}, func(ts []string, t *TransactionNudge, i int) []string {
		if !slices.Contains(ts, t.TAG.ApplicationID) {
			ts = append(ts, t.TAG.ApplicationID)
		}
		return ts
	})

	var error_ error = nil
	for _, application_id := range application_ids {
		if application_id != "" {
			languages, urlAppId := e.nudgeExporter.SendTransactions(application_id)

			if e.nudgeExporter.Parent.Console.Verbosity.String() == "Detailed" {
				fmt.Println(fmt.Sprintf("Transaction[%v] , language=%v , app_id=%v", len(e.nudgeExporter.Rawdata.Transactions), languages, application_id))
			}

			var err error = nil
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

			if e.nudgeExporter.Parent.RecordCollecte {
				e.nudgeExporter.FlushRawdataToFileDisk(request)
			}

			if urlAppId != nil {
				errX := e.export(ctx, *urlAppId, request, e.tracesPartialSuccessHandler)
				if error_ == nil && errX != nil {
					error_ = errX
				}
			} else {
				errX := e.export(ctx, e.tracesURL, request, e.tracesPartialSuccessHandler)
				if error_ == nil && errX != nil {
					error_ = errX
				}
			}
		}
	}

	return error_
}

// Equivalent du Object.keys en JS
func GetKeys(obj pcommon.Map) []string {
	keys := make([]string, 0, obj.Len())
	for key := range obj.Range {
		keys = append(keys, key)
	}
	return keys
}

func GetNameType(attributes pcommon.Map) (string, int) {

	keys := GetKeys(attributes)
	// express.name , koa.name
	kNames := extensionlinux.Reduce(keys, []string{}, func(a []string, e string, i int) []string {
		if e[len(e)-5:] == ".name" {
			return append(a, strings.Replace(e, ".name", "", 1))
		}
		return a
	})
	// express.type , koa.type
	kTypes := extensionlinux.Reduce(keys, []string{}, func(a []string, e string, i int) []string {
		if e[len(e)-5:] == ".type" {
			return append(a, strings.Replace(e, ".type", "", 1))
		}
		return a
	})
	name, b := extensionlinux.Find(kNames, func(t string) bool {
		_, i := extensionlinux.Find(kTypes, func(u string) bool {
			return u == t
		})
		return i >= 0
	})

	/*
		let keys = Object.keys(attributes);
		// express.name , koa.name
		let kNames = keys.reduce((a, e) => {
		if (e.endsWith('.name')) a.push(e.replace('.name',''));
		return a;
		}, new Array());
		// express.type , koa.type
		let kTypes = keys.reduce((a, e) => {
		if (e.endsWith('.type')) a.push(e.replace('.type',''));
		return a;
		}, new Array());
		let name = kNames.find(t => kTypes.find(u => u == t));
	*/

	return name, b
}

func GetInstrumentType(scope pcommon.InstrumentationScope) *string {

	name := scope.Name()
	names := strings.Split(name, "/")
	if len(names) >= 2 {
		names1 := strings.Split(names[1], "-")
		names1 = names1[1:]
		name = strings.Join(names1, "-")
		name = extensionlinux.ToUpperCase1(name)
		return extensionlinux.StringPtr(name)
	}

	return nil

	/*
		if (typeof sps['instrumentationLibrary'] != 'undefined' && typeof sps['instrumentationLibrary'].name != 'undefined')
		{
		let name = sps['instrumentationLibrary']['name'];
		let names = name.split('/');
		if (names.length >= 2)
		{
			names = names[1].split('-');
			names.shift();
			name = names.join('-');
			name = toUpperCase1.call(name);
			return name;
		}
		}

		return undefined;
	*/

}

func GetUuid(name string) uuid.UUID {
	// Utilisation d'un namespace (par exemple, NamespaceDNS)
	namespace := uuid.NameSpaceDNS

	// Générer un UUID v5 basé sur la chaîne "atakama"
	uuidFromString := uuid.NewMD5(namespace, []byte(name)) // ou uuid.NewSHA1()

	return uuidFromString
}

func (This *NudgeExporter) AddAllHeaders(sps SpanX) {
	attributes := sps.Attributes()
	keys := GetKeys(attributes)

	keysResponse := extensionlinux.Filter(keys, func(k string) bool {
		return extensionlinux.StartsWith(k, "http.request.header.") // || k.startsWith('http.request.');    // SemanticAttributes.HTTP_RESPONSE_CONTENT_LENGTH_UNCOMPRESSED
	})

	for _, kr := range keysResponse {
		headerName := strings.Replace(kr, "http.request.header.", "", -1)
		headerName = strings.ReplaceAll(headerName, "_", "-")

		if Headers[headerName] != "" {
			headerName = Headers[headerName]
		} else {
		}

		headerNameT := strings.Split(headerName, "-")
		headerNameT = extensionlinux.Map(headerNameT, func(e string) string {
			return extensionlinux.ToUpperCase1(e)
		})
		headerNameS := strings.Join(headerNameT, "-")

		if true /*|| ["Content-Type"].includes()*/ {
			kv := &pdata.KeyValue{} //new proto.org.nudge.buffer.KeyValue();
			kv.Reset()
			kv.Key = extensionlinux.StringPtr(headerNameS)

			if v, b := attributes.Get(kr); b { // (Array.isArray(attributes[kr]) ?
				switch v.AsRaw().(type) {
				case []any:
					v1 := extensionlinux.Map(v.Slice().AsRaw(), func(e any) string {
						return fmt.Sprintf("%v", e)
					})
					v1S := strings.Join(v1, ",")
					kv.Value = extensionlinux.StringPtr(v1S)

				default:
					v, b := attributes.Get(kr)
					if b {
						kv.Value = extensionlinux.StringPtr(v.AsString())
					} else {
						kv.Value = extensionlinux.StringPtr("?")
					}
				}

				pkv := &pdata.Param{}
				pkv.Reset()
				pkv.Key = kv.Key
				pkv.Value = kv.Value

				This.Transaction.Headers = append(This.Transaction.GetHeaders(), pkv)
			}
		}
	}
}

func (This *NudgeExporter) AddAllExtendedcodes(sps SpanX) {
	attributes := sps.Attributes()
	keys := GetKeys(attributes)

	keyValues := This.Transaction.GetExtendedCodes() //getExtendedcodesList();
	/*
		  if (keyValues.findIndex(e => e.getKey() == "OTEL.traceId") < 0)
		  {
			if (sps._spanContext && sps._spanContext.traceId)
			{
			  let kv = new proto.org.nudge.buffer.KeyValue();
			  kv.setKey("OTEL.traceId");
			  kv.setValue(sps._spanContext.traceId.toUpperCase());
			  This.Transaction.addExtendedcodes(kv);
			}
			if (sps._spanContext && sps._spanContext.spanId)
			{
			  let kv = new proto.org.nudge.buffer.KeyValue();
			  kv.setKey("OTEL.spanId");
			  kv.setValue(sps._spanContext.spanId.toUpperCase());
			  This.Transaction.addExtendedcodes(kv);
			}
		  }
	*/

	This.Transaction.InsertExtendedCode(string(semconv.HTTPMethodKey), string(semconv.HTTPMethodKey), attributes)

	filterHeader := This.Parent.FilterHeader
	headers := []string{"http.request.header.", "http.response.header."}
	for _, att := range keys {
		for _, header := range headers {
			if extensionlinux.StartsWith(att, header) && (len(filterHeader) == 0 || extensionlinux.FindIndex(filterHeader, func(e string) bool { return strings.ToLower(header) == strings.ToLower(e) }) >= 0) {
				keyValue := &pdata.KeyValue{}
				keyValue.Reset()
				headKey := att[len(header):]
				headKeys := strings.Split(headKey, "_")
				headKeys = extensionlinux.Map(headKeys, func(e string) string { return extensionlinux.ToUpperCase1(e) })
				headKey = header + strings.Join(headKeys, "-")
				if extensionlinux.FindIndex(keyValues, func(e *pdata.KeyValue) bool { return e.GetKey() == headKey }) < 0 {
					keyValue.Key = extensionlinux.StringPtr(headKey)
					v2, _ := attributes.Get(att)
					switch v2.AsRaw().(type) {
					case []any:
						keyValue.Value = extensionlinux.StringPtr(v2.Slice().At(0).AsString())
					default:
						keyValue.Value = extensionlinux.StringPtr(v2.AsString())
					}
					This.Transaction.ExtendedCodes = append(This.Transaction.GetExtendedCodes(), keyValue)
				}
			}
		}
	}

	v, b := attributes.Get(string(semconv.HTTPUserAgentKey))
	if b {
		uaS := v.AsString()
		browserInfo := user_agent.New(uaS)
		name, version := browserInfo.Browser()

		if browserInfo != nil {
			keyValue := &pdata.KeyValue{}
			keyValue.Reset()
			keyValue.Key = extensionlinux.StringPtr("Environnement.Navigateur")
			keyValue.Value = extensionlinux.StringPtr(name + " " + version)
			This.Transaction.ExtendedCodes = append(This.Transaction.GetExtendedCodes(), keyValue)

			keyValue = &pdata.KeyValue{}
			keyValue.Reset()
			keyValue.Key = extensionlinux.StringPtr("Environnement.OS")
			keyValue.Value = extensionlinux.StringPtr(browserInfo.OSInfo().FullName)
			This.Transaction.ExtendedCodes = append(This.Transaction.GetExtendedCodes(), keyValue)

			keyValue = &pdata.KeyValue{}
			keyValue.Reset()
			keyValue.Key = extensionlinux.StringPtr("Environnement.Platform")
			keyValue.Value = extensionlinux.StringPtr(browserInfo.Platform())
			This.Transaction.ExtendedCodes = append(This.Transaction.GetExtendedCodes(), keyValue)

			keyValue = &pdata.KeyValue{}
			keyValue.Reset()
			keyValue.Key = extensionlinux.StringPtr("Environnement.Engine")
			s1, s2 := browserInfo.Engine()
			keyValue.Value = extensionlinux.StringPtr(s1 + " " + s2)
			This.Transaction.ExtendedCodes = append(This.Transaction.GetExtendedCodes(), keyValue)
		}
	}

	v, b = attributes.Get(string(semconv.NetPeerIPKey))
	_, i2 := extensionlinux.Find(keyValues, func(e *pdata.KeyValue) bool { return e.GetKey() == "http.clientip" })
	if i2 < 0 && b {
		clientip := v.AsString()
		rgx := regexp.MustCompile(`:(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})`)
		matches := rgx.FindAllStringSubmatch(clientip, -1)

		if len(matches) > 0 {
			keyValue := &pdata.KeyValue{}
			keyValue.Reset()
			keyValue.Key = extensionlinux.StringPtr("http.clientip")
			keyValue.Value = extensionlinux.StringPtr(matches[0][0][1:])
			This.Transaction.ExtendedCodes = append(This.Transaction.GetExtendedCodes(), keyValue)
			This.Transaction.UserIp = extensionlinux.StringPtr(matches[0][0][1:])

			keyValue = &pdata.KeyValue{}
			keyValue.Reset()
			keyValue.Key = extensionlinux.StringPtr("http.clientip6")
			keyValue.Value = extensionlinux.StringPtr(clientip[0 : len(clientip)-len(matches[0][0])])
			This.Transaction.ExtendedCodes = append(This.Transaction.GetExtendedCodes(), keyValue)
		} else {
			keyValue := &pdata.KeyValue{}
			keyValue.Reset()
			keyValue.Key = extensionlinux.StringPtr("http.clientip")
			keyValue.Value = extensionlinux.StringPtr(v.AsString())
			This.Transaction.ExtendedCodes = append(This.Transaction.GetExtendedCodes(), keyValue)
			This.Transaction.UserIp = keyValue.Value
		}
	}

	v, b = attributes.Get(string(semconv.HTTPStatusCodeKey))
	_, i2 = extensionlinux.Find(keyValues, func(e *pdata.KeyValue) bool { return e.GetKey() == "http.status" })
	if i2 < 0 && b {
		keyValue := &pdata.KeyValue{}
		keyValue.Reset()
		keyValue.Key = extensionlinux.StringPtr("http.status")
		keyValue.Value = extensionlinux.StringPtr(v.AsString())
		This.Transaction.ExtendedCodes = append(This.Transaction.GetExtendedCodes(), keyValue)
	}

}

func (This *NudgeExporter) AddAllExtendedcodesLayerDetails(sps SpanX, keyValues *[]*pdata.KeyValue) {
	attributes := sps.Attributes()
	keys := GetKeys(attributes)

	filterHeader := This.Parent.FilterHeader
	headers := []string{"http.request.header.", "http.response.header."}
	for _, att := range keys {
		for _, header := range headers {
			if extensionlinux.StartsWith(att, header) && (len(filterHeader) == 0 || extensionlinux.FindIndex(filterHeader, func(e string) bool { return strings.ToLower(header) == strings.ToLower(e) }) >= 0) {
				keyValue := &pdata.KeyValue{}
				keyValue.Reset()
				headKey := att[len(header):]
				headKeys := strings.Split(headKey, "_")
				headKeys = extensionlinux.Map(headKeys, func(e string) string { return extensionlinux.ToUpperCase1(e) })
				headKey = header + strings.Join(headKeys, "-")
				if extensionlinux.FindIndex(*keyValues, func(e *pdata.KeyValue) bool { return e.GetKey() == headKey }) < 0 {
					keyValue.Key = extensionlinux.StringPtr(headKey)
					v2, _ := attributes.Get(att)
					switch v2.AsRaw().(type) {
					case []any:
						keyValue.Value = extensionlinux.StringPtr(v2.Slice().At(0).AsString())
					default:
						keyValue.Value = extensionlinux.StringPtr(v2.AsString())
					}
					*keyValues = append(*keyValues, keyValue)
				}
			}
		}
	}

	v, b := attributes.Get(string(semconv.NetPeerIPKey))
	_, i2 := extensionlinux.Find(*keyValues, func(e *pdata.KeyValue) bool { return e.GetKey() == "http.clientip" })
	if i2 < 0 && b {
		clientip := v.AsString()
		rgx := regexp.MustCompile(`:(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})`)
		matches := rgx.FindAllStringSubmatch(clientip, -1)

		if len(matches) > 0 {
			keyValue := &pdata.KeyValue{}
			keyValue.Reset()
			keyValue.Key = extensionlinux.StringPtr("http.clientip")
			keyValue.Value = extensionlinux.StringPtr(matches[0][0][1:])
			*keyValues = append(*keyValues, keyValue)

			keyValue = &pdata.KeyValue{}
			keyValue.Reset()
			keyValue.Key = extensionlinux.StringPtr("http.clientip6")
			keyValue.Value = extensionlinux.StringPtr(clientip[0 : len(clientip)-len(matches[0][0])])
			*keyValues = append(*keyValues, keyValue)
		} else {
			keyValue := &pdata.KeyValue{}
			keyValue.Reset()
			keyValue.Key = extensionlinux.StringPtr("http.clientip")
			keyValue.Value = extensionlinux.StringPtr(v.AsString())
			*keyValues = append(*keyValues, keyValue)
		}
	}

	v, b = attributes.Get(string(semconv.HTTPStatusCodeKey))
	_, i2 = extensionlinux.Find(*keyValues, func(e *pdata.KeyValue) bool { return e.GetKey() == "http.status" })
	if i2 < 0 && b {
		keyValue := &pdata.KeyValue{}
		keyValue.Reset()
		keyValue.Key = extensionlinux.StringPtr("http.status")
		keyValue.Value = extensionlinux.StringPtr(v.AsString())
		*keyValues = append(*keyValues, keyValue)
	}

}

func (This *NudgeExporter) TracesExportRequest2Rawdata(tr *ptracenudge.ExportRequest) {

	resource := Resource{}

	traces := tr.Traces()

	rss := traces.ResourceSpans()
	for iRss := range rss.Len() {
		rs := rss.At(iRss)
		attributesRS := rs.Resource().Attributes()

		if v, b := attributesRS.Get(string(semconv.TelemetrySDKLanguageKey)); b {
			resource.Telemetry.Language = v.AsString()
		}
		if v, b := attributesRS.Get(string(semconv.TelemetrySDKVersionKey)); b {
			resource.Telemetry.Version = v.AsString()
		}
		if v, b := attributesRS.Get(string(semconv.TelemetrySDKNameKey)); b {
			resource.Telemetry.Name = v.AsString()
		}
		if v, b := attributesRS.Get(string(semconv.ServiceNameKey)); b {
			resource.Telemetry.ServiceName = v.AsString()
		}
		if v, b := attributesRS.Get(string(semconv.HostNameKey)); b {
			resource.Host.Name = v.AsString()
		}
		if v, b := attributesRS.Get(string(semconv.HostArchKey)); b {
			resource.Host.Arch = v.AsString()
		}
		if v, b := attributesRS.Get(string(semconv.HostIDKey)); b {
			resource.Host.Id = v.AsString()
		}

		fmt.Println(fmt.Sprintf("%v, %v", resource.Telemetry.ToString(), resource.Host.ToString()))

		sss := rs.ScopeSpans()
		for iSss := range sss.Len() {
			ss := sss.At(iSss)
			spans := ss.Spans()
			scope := ss.Scope()
			attributesS := scope.Attributes()
			attributesS = attributesS

			INSTRUMENT := GetInstrumentType(scope)
			for iSps := range spans.Len() {
				sps := newSpanX(spans.At(iSps))
				//parentSpanID := sps.ParentSpanID().String()
				//traceID := sps.TraceID().String()
				//spanID := sps.SpanID().String()
				attributes := sps.Attributes()

				keys := GetKeys(attributes)
				HTTP := extensionlinux.FindIndex(keys, func(e string) bool { return extensionlinux.StartsWith(e, "http.") }) >= 0   //keys.findIndex(k => { return k.startsWith('http.'); }) >= 0;
				NET := extensionlinux.FindIndex(keys, func(e string) bool { return extensionlinux.StartsWith(e, "net.") }) >= 0     //keys.findIndex(k => { return k.startsWith('net.'); }) >= 0;
				DB := extensionlinux.FindIndex(keys, func(e string) bool { return extensionlinux.StartsWith(e, "db.") }) >= 0       //keys.findIndex(k => { return k.startsWith('db.'); }) >= 0;
				GQL := extensionlinux.FindIndex(keys, func(e string) bool { return extensionlinux.StartsWith(e, "graphql.") }) >= 0 //keys.findIndex(k => { return k.startsWith('graphql.'); }) >= 0;
				var SERVEUR *string = nil
				x, b := GetNameType(attributes)
				if b >= 0 {
					SERVEUR = extensionlinux.StringPtr(x)
				}

				uuid := GetUuid(sps.TraceID().String()).String()
				This.Transaction, _ = extensionlinux.Find(This.Transactions, func(t *TransactionNudge) bool {
					return t.GetUuid() == uuid
				})
				if This.Transaction == nil {
					This.Transaction = newTransactionNudge()
					This.Transactions = append(This.Transactions, This.Transaction)

					This.Transaction.TAG.Correlationid = 0
					// Il n'y a pas de traitement intermédiaire avant le profiling donc je désactive le Urlid_WAITANDSEND
					This.Transaction.TAG.Urlid = Urlid_WAITANDSEND
					This.Transaction.TAG.Serveur = nil

					This.Transaction.Status = pdata.Transaction_OK.Enum()
					This.Transaction.Uuid = extensionlinux.StringPtr(uuid)
					attributesX := pcommon.NewMap()
					attributes.CopyTo(attributesX)
					attributesX.PutStr(pdata.SemanticAttributesX["UUID"], string(*This.Transaction.Uuid)) // this.urlDictionary.getUUID(sps._spanContext.traceId);
					attributes = attributesX
					if This.Parent.SessionId != "" {
						This.Transaction.SessionId = extensionlinux.StringPtr(This.Parent.SessionId)
					} else {
						s := time.Now().Format("2006-01-02 15:04:05")
						This.Transaction.SessionId = extensionlinux.StringPtr(GetUuid(s).String())
					}
				}

				if v, b := attributes.Get(Nudge_application_id); b {
					if This.Transaction.TAG.ApplicationID == "" || This.Transaction.TAG.ApplicationID != v.AsString() {
						This.Transaction.TAG.ApplicationID = v.AsString()
						This.Transaction.TAG.UrlAppId = This.Attributes2URL(This.Parent, attributes)
					} else {
						// Normalement c'est une erreur dans le flux des attributs ou alors application_id n'a pas pu etre determiné par "attributes/add_app_id_java:"
					}
				} else if This.Transaction.TAG.ApplicationID == "" || This.Transaction.TAG.ApplicationID != This.Parent.NudgeHTTPClientConfig.AppID {
					This.Transaction.TAG.ApplicationID = This.Parent.NudgeHTTPClientConfig.AppID
					s, err := composeSignalURL(This.Parent, This.Parent.TracesEndpoint, This.Parent.NudgeHTTPClientConfig.PathCollect, This.Parent.NudgeHTTPClientConfig.AppID)
					if err == nil {
						This.Transaction.TAG.UrlAppId = extensionlinux.StringPtr(s)
					}
				} else {
					// Normalement c'est une erreur dans le flux des attributs ou alors application_id n'a pas pu etre determiné par "attributes/add_app_id_java:"
				}

				/*
					let uuid = getUuid(sps['_spanContext']['traceId']);
					this.transaction = this.transactions.find(t => t.getUuid() == uuid);
					if (!this.transaction)
					{
					this.transaction = new proto.org.nudge.buffer.Transaction();
					this.transactions.push(this.transaction);

					this.transaction.TAG = {
						correlationid : 0,
						urlid : 1,
						serveur : undefined,
					};
					// La transaction est marquée comme non envoyée et supprimée de la liste des transactions
					// this.transaction.setUrlid(2);

					this.transaction.setUuid(uuid);
					// Creation ou pas du UUID pour le traceId de la session de l'URL a utiliser dans threadInfos
					// this.transaction.setUuid(this.urlDictionary.makeUUID(getUrl(attributes)));

					attributes[SemanticAttributes.UUID] = this.transaction.getUuid(); // this.urlDictionary.getUUID(sps._spanContext.traceId);
					this.transaction.setSessionid( global.configurationAgentNodeJS.Nudge?.sessionId );
					}
				*/
				if NET {
					//n := 0
				}

				if (SERVEUR != nil || HTTP) && ((sps.Kind() == ptrace.SpanKindInternal /*|| sps.Kind() == ptrace.SpanKindUnspecified*/) || (sps.ParentSpanID().IsEmpty())) {
					/*
						TYPE
						  SQL_REQUEST:2
						  SUB_TRANSACTION:1
						  TRANSACTION:0

						STATUS
						  KO:1
						  OK:0

						METHOD
						  CONNECT:0
						  DELETE:1
						  GET:2
						  HEAD:3
						  OPTIONS:4
						  POST:5
						  PUT:6
						  TRACE:7

						peer = distant
						host = local (serveur)
						NET
						  "net.host.ip", '::1'
						  "net.host.name", 'localhost'
						  "net.host.port", 8080
						  "net.peer.ip", '::1'
						  "net.peer.port", 49254
						  "net.transport", 'ip_tcp'
					*/

					//this.transaction.setUpstreamagentid();
					This.Transaction.UpstreamTxId = extensionlinux.StringPtr(sps.TraceID().String()) //sps?._spanContext?.traceId);
					This.Transaction.UpstreamCorrelationId = extensionlinux.NumberPtr(This.Transaction.TAG.Correlationid)
					This.Transaction.TAG.Correlationid++

					peer := ""
					if v1, b := attributes.Get(string(semconv.NetPeerNameKey)); b { //SemanticAttributes.NET_PEER); b {
						v2, b := attributes.Get(string(semconv.NetHostPortKey))
						if b {
							peer = v1.AsString() + v2.AsString()
						} else {
							peer = v1.AsString()
						}
					} else if v1, b := attributes.Get(string(semconv.NetPeerNameKey)); b {
						v2, b := attributes.Get(string(semconv.NetHostPortKey))
						if b {
							peer = v1.AsString() + v2.AsString()
						} else {
							peer = v1.AsString()
						}
					} else if v1, b := attributes.Get(string(semconv.NetPeerIPKey)); b {
						v2, b := attributes.Get(string(semconv.NetHostPortKey))
						if b {
							peer = v1.AsString() + v2.AsString()
						} else {
							peer = v1.AsString()
						}
					}
					peer = peer

					// Seul les headers REQUEST sont pertinents
					/*
						let keysResponse = keys.filter(k => {
							return k.startsWith('http.response.header.');// || k.startsWith('http.request.');    // SemanticAttributes.HTTP_RESPONSE_CONTENT_LENGTH_UNCOMPRESSED
						});
						keysResponse.forEach(kr => {
							let headerName = kr.replace('http.response.header.','').replace(/_/gm,'-');
							headerName = Headers[headerName] != undefined ? Headers[headerName] : headerName;
							let header = new proto.org.nudge.buffer.Param();
							header.setKey(headerName);
							header.setValue(attributes[kr].toString());
							this.transaction.addHeaders(header);
						});
					*/

					This.AddAllHeaders(sps)
					This.AddAllExtendedcodes(sps)

					//let iname = name.indexOf('/');
					//name = iname >= 0 ? name.substring(iname) : name;
					if sps.ParentSpanID().IsEmpty() {
						name := sps.Name()
						This.Transaction.Code = extensionlinux.StringPtr(name)
					}

					//this.transaction.setSeg1id();
					//this.transaction.setSeg2id();
					//this.transaction.setSeg3id();
					st := sps.StartTimestamp().AsTime()
					et := sps.EndTimestamp().AsTime()
					if !st.IsZero() && This.Transaction.StartTime == nil {
						This.Transaction.StartTime = extensionlinux.NumberPtr(st.UTC().UnixMilli())
					} else if !st.IsZero() && This.Transaction.StartTime != nil {
						This.Transaction.StartTime = extensionlinux.NumberPtr(slices.Min([]int64{This.Transaction.GetStartTime(), st.UTC().UnixMilli()}))
					}
					This.Transaction.EndTime = extensionlinux.NumberPtr(slices.Max([]int64{This.Transaction.GetEndTime(), et.UTC().UnixMilli()}))

					url, b := attributes.Get(string(semconv.HTTPURLKey))
					if b {
						// Notation DEBUG
						This.Transaction.TAG.Code = This.Transaction.GetCode()
						This.Transaction.TAG.TraceId = sps.TraceID().String() //sps._spanContext.traceId

						paramsX := strings.SplitN(url.AsString(), "?", 2) // .split('?', 2);
						if len(paramsX) >= 1 {
							param := &pdata.Param{}
							param.Reset()
							param.Key = extensionlinux.StringPtr("url")
							param.Value = extensionlinux.StringPtr(paramsX[0])
							param.Type = extensionlinux.StringPtr("string")

							This.Transaction.Params = append(This.Transaction.GetParams(), param)
						}
						if len(paramsX) >= 2 {
							paramsX := strings.SplitN(paramsX[1], "&", 2)
							params := extensionlinux.Map(paramsX, func(e string) *pdata.Param {
								t := strings.SplitN(e, "=", 2)
								if len(t) == 1 {
									t = append(t, "")
								}
								r := &pdata.Param{}
								r.Reset()
								r.Key = extensionlinux.StringPtr(t[0])
								r.Value = extensionlinux.StringPtr(t[1])
								r.Type = extensionlinux.StringPtr("string")

								return r
							})

							for _, kv := range params {
								param := &pdata.Param{}
								param.Reset()
								param.Key = kv.Key
								param.Value = kv.Value
								param.Type = extensionlinux.StringPtr("string")

								This.Transaction.Params = append(This.Transaction.GetParams(), param)
							}
						}
					}

					if v, b := attributes.Get(string(semconv.HTTPStatusCodeKey)); b {
						This.Transaction.RespStatusCode = extensionlinux.NumberPtr(int32(v.Int())) // .setRespstatuscode(v);
					}

					/*
						if (attributes && (includesOneField(attributes, [SemanticAttributes.HTTP_ERROR_NAME, SemanticAttributes.HTTP_ERROR_MESSAGE, /*SemanticAttributes.HTTP_STATUS_CODE, */
					/*SemanticAttributes.HTTP_STATUS_TEXT,*+/ SemanticAttributes.HTTP_STACKTRACE]) || (attributes[SemanticAttributes.HTTP_STATUS_CODE] != undefined) && attributes[SemanticAttributes.HTTP_STATUS_CODE] >= 400 ))

					 */
					v, _ := attributes.Get(pdata.SemanticAttributesX["HTTP_STATUS_CODE"])
					keys := GetKeys(attributes)
					if (extensionlinux.IncludesOneField(keys, []string{pdata.SemanticAttributesX["HTTP_ERROR_NAME"], pdata.SemanticAttributesX["HTTP_ERROR_MESSAGE"], /*SemanticAttributes.HTTP_STATUS_CODE, */
						/*SemanticAttributes.HTTP_STATUS_TEXT,*/ pdata.SemanticAttributesX["HTTP_STACKTRACE"]}) != nil || extensionlinux.IncludesOneField(keys, []string{string(semconv.HTTPStatusCodeKey)}) != nil) && v.Int() >= 400 {
						error_ := newError()
						//error_.setServerid();

						if v, b := attributes.Get(pdata.SemanticAttributesX["HTTP_STATUS_CODE"]); b {
							error_.Code = extensionlinux.StringPtr("Http status: " + v.AsString())
						}

						error_.StartTime = extensionlinux.NumberPtr(int64(sps.StartTimestamp()))

						if v, b := attributes.Get(pdata.SemanticAttributesX["HTTP_ERROR_MESSAGE"]); b {
							v2, b2 := attributes.Get(pdata.SemanticAttributesX["HTTP_ERROR_NAME"])
							if b2 {
								error_.Message = extensionlinux.StringPtr(v.AsString() + " - " + v2.AsString())
							} else {
								error_.Message = extensionlinux.StringPtr(v.AsString())
							}
						} else if v, b := attributes.Get(pdata.SemanticAttributesX["HTTP_STATUS_CODE"]); b && v.Int() >= 400 {
							v2, b2 := attributes.Get(pdata.SemanticAttributesX["HTTP_STATUS_TEXT"])
							v3, b3 := attributes.Get(pdata.SemanticAttributesX["HTTP_ERROR_NAME"])
							if b2 {
								error_.Message = extensionlinux.StringPtr(v2.AsString())
							}
							if b3 {
								error_.Message = extensionlinux.StringPtr(*error_.Message + " - " + v3.AsString())
							}
						}
						/*
							if (typeof attributes[SemanticAttributes.HTTP_STACKTRACE] != 'undefined')
							{
							  let stack = attributes[SemanticAttributes.HTTP_STACKTRACE].join('\n');
							  error.setStacktrace(stack.replace(/\n\s*at\s* /igm,'#'));
							  error.setJvmstacktrace(stack.replace(/\n/gm,'\r\n\t'));
							}
						*/

						This.Transaction.ErrorsX = append(This.Transaction.GetErrors(), error_)
					}

					// Erreurs remontées par serveur KOA mais pas par EXPRESS car le span se termine avant le catch de l'erreur
					events := sps.Events()
					for i := range events.Len() {
						evnt := events.At(i)
						if evnt.Name() == "exception" {
							attr := evnt.Attributes()
							error_ := newError()
							//error_.setServerid();

							v, _ := attr.Get("exception.type")
							error_.Code = extensionlinux.StringPtr(v.AsString())
							v, _ = attr.Get("startTime")
							error_.StartTime = extensionlinux.NumberPtr(v.Int())

							v, _ = attr.Get("exception.message")
							error_.Message = extensionlinux.StringPtr(v.AsString())
							v, _ = attr.Get("exception.stacktrace")

							re := regexp.MustCompile(`(?i)\n\s*at\s*`)
							error_.Stacktrace = extensionlinux.StringPtr(re.ReplaceAllString(v.AsString(), "#"))
							re = regexp.MustCompile(`\n`)
							error_.JvmStacktrace = extensionlinux.StringPtr(re.ReplaceAllString(v.AsString(), "\r\n\t"))

							This.Transaction.ErrorsX = append(This.Transaction.GetErrors(), error_)
							This.Transaction.Status = pdata.Transaction_KO.Enum()
						}
					}

					// sps.status.code = 2 pour des erreurs SQL et 1 pour des erreurs HTTP
					// un responseCode = 202 ==> sps.status.code = 1 or il n'y a pas d'erreur
					_, b = attributes.Get(pdata.SemanticAttributesX["HTTP_ERROR_NAME"])
					_, b2 := attributes.Get(pdata.SemanticAttributesX["HTTP_ERROR_MESSAGE"])
					if b || b2 {
						ts := pdata.Transaction_Status(0)
						This.Transaction.Status = &ts
					} else if v3, b3 := attributes.Get(pdata.SemanticAttributesX["HTTP_STATUS_CODE"]); b3 {
						if v3.Int() >= 400 || v3.Int() <= 0 {
							This.Transaction.Status = pdata.Transaction_KO.Enum()
						}
						//else
						//  this.transaction.setStatus( proto.org.nudge.buffer.Transaction.Status.OK );
					} else {
						if sps.Status().Code() != ptrace.StatusCodeUnset {
							//This.Transaction.Status = pdata.Transaction_OK.Enum()
						}
					}

					v, b = attributes.Get(string(semconv.HTTPMethodKey))
					if b {
						This.Transaction.MethodName = extensionlinux.StringPtr(v.AsString())
					}

					v, b = attributes.Get(string(semconv.HTTPUserAgentKey))
					if b {
						This.Transaction.UserAgent = extensionlinux.StringPtr(v.AsString())

						ua := This.Rawdata.GetUserAgent()
						if ua == nil {
							userAgentDictionary := &pdata.Dictionary{}
							userAgentDictionary.Reset()
							This.Rawdata.UserAgent = userAgentDictionary
							ua = userAgentDictionary
						}
						/* userAgentDictionary
						array = (1) [Array(0)]
						arrayIndexOffset_ = -1
						convertedPrimitiveFields_ = {}
						messageId_ = undefined
						pivot_ = 1.7976931348623157e+308
						wrappers_ = null
						[[Prototype]] = jspb.Message

						OU

						0 = (1) [Array(2)]
						0 = (2) ['Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWe…HTML, like Gecko) Chrome/135.0.0.0 Safari/537.36', 0]
						0 = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/135.0.0.0 Safari/537.36'
						1 = 0
						*/
						if ua != nil {
							r := extensionlinux.Map(ua.GetDictionary(), func(e *pdata.Dictionary_DictionaryEntry) string {
								return *e.Name
							})
							// TRADUCION A VERIFIER : ua.array[0].map(e => e[0]) !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!
							//	if (!ua.array[0].map(e => e[0]).includes(attributes[SemanticAttributes.HTTP_USER_AGENT])) {
							if !slices.Contains(r, v.AsString()) {
								id := len(ua.GetDictionary()) //
								aEntry := &pdata.Dictionary_DictionaryEntry{}
								aEntry.Reset()
								aEntry.Name = extensionlinux.StringPtr(v.AsString())
								aEntry.Id = extensionlinux.NumberPtr(int32(id))
								ua.Dictionary = append(ua.GetDictionary(), aEntry)

								This.Transaction.UseragentID = extensionlinux.NumberPtr(int32(id)) // Correspond au navigateur
							} else {
								//dua := ua.array[0].find(e => e[0] == attributes[SemanticAttributes.HTTP_USER_AGENT]);
								dua, _ := extensionlinux.Find(ua.GetDictionary(), func(e *pdata.Dictionary_DictionaryEntry) bool {
									return e.GetName() == v.AsString()
								})
								This.Transaction.UseragentID = extensionlinux.NumberPtr(dua.GetId()) //dua[1]
							}
						}
					}

					// this.transaction.setErrorsList();
					// this.transaction.setDbcnxcount();
					// this.transaction.setDbcnxavg();
					// this.transaction.setDbcnxmin();
					// this.transaction.setDbcnxmax();
					// this.transaction.setDbquerycount();
					// this.transaction.setDbqueryavg();
					// this.transaction.setDbquerymin();
					// this.transaction.setDbquerymax();
					// this.transaction.setDbfetchcount();
					// this.transaction.setSqlrequestsList();
					// this.transaction.setDbcommitcount();
					// this.transaction.setDbcommitavg();
					// this.transaction.setDbcommitmin();
					// this.transaction.setDbcommitmax();
					// this.transaction.setDbrollbackcount();
					// this.transaction.setDbrollbackavg();

					// let sa = new proto.org.nudge.buffer.SessionActivity();

					//v, b = attributes.Get(string(semconv.HTTPRouteKey))
					//if b && sps.Name() == v.AsString() {
					// La fin d'un span et lorsque la parentSpanId == undefined
					/* debut d'un appel de service : exemple -
					"http.route": "/free",
					"express.name": "/free",
					"express.type": "request_handler",
					UUID: "6d8c4362-d092-43f5-b1c8-c75cf6e39b4d",
					*/
					/* fin d'un appel de service KOA : exemple -
					"http.route": "/koa_reqhttp",
					"koa.name": "/koa_reqhttp",
					"koa.type": "router",
					UUID: "6d8c4362-d092-43f5-b1c8-c75cf6e39b4d",
					*/
					//}

					if SERVEUR != nil {
						if This.Transaction.TAG.Serveur == nil {
							This.Transaction.TAG.Serveur = newServer()

							name, _ := attributes.Get(*SERVEUR + ".name")
							This.Transaction.TAG.Serveur.Name = name.AsString()
							type_, _ := attributes.Get(*SERVEUR + ".type")
							This.Transaction.TAG.Serveur.Type = type_.AsString()
							This.Transaction.TAG.Serveur.Nom = *SERVEUR
						}

						if This.Transaction.TxType == nil {
							This.Transaction.TxType = extensionlinux.StringPtr(extensionlinux.ToUpperCase1(*SERVEUR))
						}

						if This.Transaction.TAG.Serveur.Instrument == "" {
							This.Transaction.TAG.Serveur.Instrument = *INSTRUMENT
						}
					}

					if This.Parent.WaitEndService {
						// Marquée pour attendre la fin puis être envoyé dans le rawdata si pas de parentSpanId
						if sps.ParentSpanID().IsEmpty() {
							This.Transaction.TAG.Urlid = Urlid_NOWAITANDSEND
						}
					} else { // Marquée pour ne pas attendre la fin puis être envoyé dans le rawdata
						This.Transaction.TAG.Urlid = Urlid_NOWAITANDSEND
					}

					/*
						if This.Transaction.TxType == nil && INSTRUMENT != nil {
							This.Transaction.TxType = extensionlinux.StringPtr(extensionlinux.ToUpperCase1(*INSTRUMENT))
							// Evite un ecrasement
							if This.Transaction.TAG.Serveur == nil {
								This.Transaction.TAG.Serveur = &Server{}
							}
							This.Transaction.TAG.Serveur.Instrument = *INSTRUMENT
						}
					*/

				} else if HTTP && (sps.Kind() == ptrace.SpanKindServer || !sps.ParentSpanID().IsEmpty()) {
					if sps.ParentSpanID().IsEmpty() {
						v, b := attributes.Get(string(semconv.HTTPRouteKey))
						if b {
							This.Transaction.Code = extensionlinux.StringPtr(v.AsString())
						}
						// this.sendTransaction();
						// La transaction est marquée pour etre envoyée dans le rawdata
						This.Transaction.TAG.Urlid = Urlid_NOWAITANDSEND
					}

					urlX := ""
					v, b := attributes.Get(string(semconv.NetPeerNameKey))
					v2, b2 := attributes.Get(string(semconv.NetPeerIPKey))
					v3, b3 := attributes.Get(string(semconv.NetPeerPortKey))
					if b {
						urlX = v.AsString()
						if b3 {
							urlX += ":" + v3.AsString()
						}
					} else if b2 {
						urlX = v2.AsString()
						if b3 {
							urlX += ":" + v3.AsString()
						}
					}

					// Étape 1 : Split sur `://`
					parts := strings.SplitN(urlX, "://", 2)
					// Étape 2 : Split sur `:` ou `/` (regex)
					re := regexp.MustCompile(`[:\/]`)
					urlXs := re.Split(parts[0], 2)
					urlXs = urlXs

					layerType := LAYER_TYPE_HTTP

					layerHTTP, i := extensionlinux.Find(This.Transaction.GetLayers(), func(l *pdata.Layer) bool {
						return l.GetLayerName() == layerType
					})
					if i < 0 {
						layerHTTP = &pdata.Layer{}
						layerHTTP.Reset()
						This.Transaction.Layers = append(This.Transaction.GetLayers(), layerHTTP)
						layerHTTP.LayerName = extensionlinux.StringPtr(layerType) //  SemanticAttributes.LAYER_TYPE_HTTP);
						layerHTTP.Count = extensionlinux.NumberPtr(int64(0))
						layerHTTP.Errors = extensionlinux.NumberPtr(int64(0))
					}

					// Creation ou pas du UUID pour URL a utiliser dans threadInfos
					//this.transaction.setUuid(this.urlDictionary.makeUUID(url));
					//attributes[SemanticAttributes.UUID] = this.urlDictionary.getUUID(url);

					layerDetail := &pdata.LayerDetail{}
					layerDetail.Reset()
					layerDetail.Count = extensionlinux.NumberPtr(layerDetail.GetCount() + 1)

					v, b = attributes.Get(string(semconv.HTTPStatusCodeKey))
					// 1 => erreur HTTP , 2 => erreur SQL
					if sps.Status().Code() == Error_HTTP && b || v.Int() >= 400 {
						error_ := newError()

						//if (typeof attributes[SemanticAttributes.HTTP_ERROR_NAME] != 'undefined')
						//  error.setCode(attributes[SemanticAttributes.HTTP_ERROR_NAME]);

						error_.Code = extensionlinux.StringPtr(sps.Status().Code().String())
						error_.StartTime = extensionlinux.NumberPtr(sps.StartTimestamp().AsTime().UnixMilli())

						if sps.Status().Message() != "" {
							v, _ := attributes.Get(string(semconv.HTTPURLKey))
							if len(v.AsString()) > 30 {
								error_.Message = extensionlinux.StringPtr(v.AsString() + "...")
							} else {
								error_.Message = extensionlinux.StringPtr(v.AsString() + " : " + sps.Status().Message())
							}
						}

						v, b = attributes.Get(pdata.SemanticAttributesX["HTTP_STACKTRACE"])
						if b {
							r := extensionlinux.Map(v.Slice().AsRaw(), func(a any) string {
								b, _ := a.(string)
								return b
							})
							err := strings.Join(r, "\n")

							re := regexp.MustCompile(`(?i)\n\s*at\s*`) // (/\n\s*at\s*+/igm,'#')
							error_.Stacktrace = extensionlinux.StringPtr(re.ReplaceAllString(err, "#"))
							re = regexp.MustCompile(`\n`)
							error_.JvmStacktrace = extensionlinux.StringPtr(re.ReplaceAllString(err, "\r\n\t")) //(se.stack.replace(/\n/gm,'\r\n\t'));
						}

						This.Transaction.Status = pdata.Transaction_KO.Enum()
						This.Transaction.ErrorsX = append(This.Transaction.GetErrors(), error_)
						layerDetail.Errors = extensionlinux.NumberPtr(layerDetail.GetErrors() + 1)
						layerHTTP.Errors = extensionlinux.NumberPtr(layerHTTP.GetErrors() + 1)
					} else {
						//This.Transaction.Status = pdata.Transaction_OK.Enum()
						layerDetail.Errors = extensionlinux.NumberPtr(int64(0))
					}

					v, b = attributes.Get(string(semconv.HTTPURLKey))
					if b {
						layerDetail.Code = extensionlinux.StringPtr(v.AsString())
					}

					layerDetail.Timestamp = extensionlinux.NumberPtr(sps.StartTimestamp().AsTime().UnixMilli())
					time := sps.EndTimestamp().AsTime().UnixMilli() - sps.StartTimestamp().AsTime().UnixMilli()
					layerDetail.Time = extensionlinux.NumberPtr(time)

					InsertExtCode(string(semconv.HTTPTargetKey), string(semconv.HTTPTargetKey), attributes, layerDetail)
					InsertExtCode(pdata.SemanticAttributesX["HTTP_STATUS_TEXT"], pdata.SemanticAttributesX["HTTP_STATUS_TEXT"], attributes, layerDetail)
					InsertExtCode(string(semconv.HTTPFlavorKey), string(semconv.HTTPFlavorKey), attributes, layerDetail)
					InsertExtCode(string(semconv.HTTPMethodKey), string(semconv.HTTPMethodKey), attributes, layerDetail)
					InsertExtCode(string(semconv.NetPeerNameKey), string(semconv.NetPeerNameKey), attributes, layerDetail)
					InsertExtCode(string(semconv.NetPeerPortKey), string(semconv.NetPeerPortKey), attributes, layerDetail)

					This.AddAllExtendedcodesLayerDetails(sps, &layerDetail.ExtCodes)

					layerHTTP.Calls = append(layerHTTP.GetCalls(), layerDetail)
					layerHTTP.Count = extensionlinux.NumberPtr(layerHTTP.GetCount() + layerDetail.GetCount())
					layerHTTP.Time = extensionlinux.NumberPtr(layerHTTP.GetTime() + layerDetail.GetTime())
					layerHTTP.Max = extensionlinux.NumberPtr(slices.Max([]int64{layerHTTP.GetMax(), layerDetail.GetTime()}))
					layerHTTP.Min = extensionlinux.NumberPtr(slices.Min([]int64{layerHTTP.GetMin(), layerDetail.GetTime()}))

				} else if DB || GQL {
					if sps.ParentSpanID().IsEmpty() {
						This.Transaction.Code = extensionlinux.StringPtr(sps.Name())
						// this.sendTransaction();
						// La transaction est marquée pour etre envoyée dans le rawdata
						This.Transaction.TAG.Urlid = Urlid_NOWAITANDSEND
					}

					layerType := LAYER_TYPE_SQL

					layerSQL, i := extensionlinux.Find(This.Transaction.GetLayers(), func(l *pdata.Layer) bool {
						return l.GetLayerName() == layerType
					})
					if i < 0 {
						layerSQL = &pdata.Layer{}
						layerSQL.Reset()
						This.Transaction.Layers = append(This.Transaction.GetLayers(), layerSQL)
						layerSQL.LayerName = extensionlinux.StringPtr(layerType) //  SemanticAttributes.LAYER_TYPE_HTTP);
						layerSQL.Count = extensionlinux.NumberPtr(int64(0))
						layerSQL.Errors = extensionlinux.NumberPtr(int64(0))
					}

					//let url = getUrl(attributes);
					//this.transaction.setUrl(url);
					// Creation ou pas du UUID pour URL a utiliser dans threadInfos
					//this.transaction.setUuid(this.urlDictionary.makeUUID(url));
					//attributes[SemanticAttributes.UUID] = this.urlDictionary.getUUID(url);

					layerDetail := &pdata.LayerDetail{}
					layerDetail.Reset()
					layerDetail.Count = extensionlinux.NumberPtr(layerDetail.GetCount() + 1)

					// 1 => erreur HTTP , 2 => erreur SQL
					if sps.Status().Code() == Error_SQL {
						error_ := newError()
						//error.setServerid();

						//if (typeof attributes[SemanticAttributes.HTTP_ERROR_NAME] != 'undefined')
						//  error.setCode(attributes[SemanticAttributes.HTTP_ERROR_NAME]);
						v, b := attributes.Get(string(semconv.DBSystemKey))
						if b {
							error_.Code = extensionlinux.StringPtr("SQL - " + v.AsString())
						}

						error_.StartTime = extensionlinux.NumberPtr(sps.StartTimestamp().AsTime().UnixMilli())

						if sps.Status().Message() == "" {
							v, _ := attributes.Get(string(semconv.DBStatementKey))
							if len(v.AsString()) > 40 {
								error_.Message = extensionlinux.StringPtr(v.AsString()[:40] + "...")
							} else {
								error_.Message = extensionlinux.StringPtr(v.AsString() + " : " + sps.Status().Message())
							}
						}

						v, b = attributes.Get(pdata.SemanticAttributesX["HTTP_STACKTRACE"])
						if b {
							r := extensionlinux.Map(v.Slice().AsRaw(), func(a any) string {
								b, _ := a.(string)
								return b
							})
							err := strings.Join(r, "\n")

							re := regexp.MustCompile(`(?i)\n\s*at\s*`) // (/\n\s*at\s*+/igm,'#')
							error_.Stacktrace = extensionlinux.StringPtr(re.ReplaceAllString(err, "#"))
							re = regexp.MustCompile(`\n`)
							error_.JvmStacktrace = extensionlinux.StringPtr(re.ReplaceAllString(err, "\r\n\t")) //(se.stack.replace(/\n/gm,'\r\n\t'));
						}

						This.Transaction.Status = pdata.Transaction_KO.Enum()
						This.Transaction.ErrorsX = append(This.Transaction.GetErrors(), error_)
						layerDetail.Errors = extensionlinux.NumberPtr(layerDetail.GetErrors() + 1)
						layerSQL.Errors = extensionlinux.NumberPtr(layerSQL.GetErrors() + 1)
					} else {
						//This.Transaction.Status = pdata.Transaction_OK.Enum()
						layerDetail.Errors = extensionlinux.NumberPtr(layerDetail.GetErrors())
					}

					v, b := attributes.Get(string(semconv.DBStatementKey))
					if b {
						layerDetail.Code = extensionlinux.StringPtr(v.AsString())
					}

					layerDetail.Timestamp = extensionlinux.NumberPtr(sps.StartTimestamp().AsTime().UnixMilli())
					time := sps.EndTimestamp().AsTime().UnixMilli() - sps.StartTimestamp().AsTime().UnixMilli()
					layerDetail.Time = extensionlinux.NumberPtr(time)

					// layerDetail.setValuesList();
					/*
						  var serverJDBC = attributes[SemanticAttributes.DB_CONNECTION_STRING].matchAll(/^jdbc:([^:]+):.*$/g);
						  serverJDBC = serverJDBC.next();
						  serverJDBC = serverJDBC && Array.isArray(serverJDBC.value) && serverJDBC.value.length >= 2 ? serverJDBC.value[1] : undefined;

						  resource: {
							_attributes: {
							  "service.name": "unknown_service:node",
							  "telemetry.sdk.language": "nodejs",
							  "telemetry.sdk.name": "opentelemetry",
							  "telemetry.sdk.version": "1.11.0",
							},
							asyncAttributesPending: false,
							_syncAttributes: {
							  "service.name": "unknown_service:node",
							  "telemetry.sdk.language": "nodejs",
							  "telemetry.sdk.name": "opentelemetry",
							  "telemetry.sdk.version": "1.11.0",
							},
							_asyncAttributesPromise: undefined,
						  }
					*/
					/* MySQL :
					  "db.system": "mysql",
					  "net.peer.name": "localhost",
					  "net.peer.port": 3306,
					  "db.connection_string": "jdbc:mysql://localhost:3306/atakama",
					  "db.name": "atakama",
					  "db.user": "root",
					  "db.statement": "SELECT * FROM licences limit 2",

					  MongoDB :
					  "db.system": "mongodb",
					  "db.name": "admin",
					  "db.mongodb.collection": "$cmd",
					  "net.host.name": "localhost",
					  "net.host.port": "27017",
					  "db.statement": "{\"listDatabases\":\"?\"}",

					  Sqlite3 :
					  {
						"db.system": "sqlite3",
						"db.statement": "SELECT * FROM Personne limit 2",
						"db.name": "C:\\Users\\frederic1\\Documents\\DEVELOPPEMENT NUDGE\\NUDGE\\ApplicationCliente\\personnes.db",
					  }

					  sqlite:
					  {
						"knex.version": "2.4.2",
						"db.system": "sqlite",
						"db.name": "C:\\Users\\frederic1\\Documents\\DEVELOPPEMENT NUDGE\\NUDGE\\Ghost\\versions\\5.65.0\\content\\data\\ghost-dev.db",
						"db.statement": "select * from sqlite_master where type = 'table' and name = ?",
					  }
					*/

					if INSTRUMENT != nil {
						switch *INSTRUMENT {
						case "Graphql":
							// ATTR_GRAPHQL_OPERATION_NAME
							_, b = attributes.Get("graphql.operation.type")
							if b {
								InsertExtCode("graphql.operation.type", "jdbc.name", attributes, layerDetail)

								connection_string := "jdbc:" + strings.ToLower(*INSTRUMENT) + "://"
								keyValue := &pdata.KeyValue{}
								keyValue.Reset()
								keyValue.Key = extensionlinux.StringPtr("jdbc.url")
								keyValue.Value = extensionlinux.StringPtr(connection_string)
								layerDetail.ExtCodes = append(layerDetail.GetExtCodes(), keyValue)

								InsertExtCode("graphql.source", string(semconv.DBStatementKey), attributes, layerDetail)
								InsertExtCode("graphql.operation.type", "jdbc.product.user", attributes, layerDetail)
							} else {
								keyValue, _ := InsertExtCode("graphql.source", string(semconv.DBSystemKey), attributes, layerDetail)
								keyValue.Value = extensionlinux.StringPtr(strings.ToLower(*INSTRUMENT))

								//InsertExtCode("graphql.source", string(semconv.DBStatementKey), attributes, layerDetail)
							}

							layerSQL.Calls = append(layerSQL.GetCalls(), layerDetail)

						default:
							v, b = attributes.Get(string(semconv.DBConnectionStringKey))
							if b {
								keyValue := &pdata.KeyValue{}
								keyValue.Reset()
								keyValue.Key = extensionlinux.StringPtr("jdbc.url")
								connection_string := v.AsString()
								if This.Parent.SendUserConnectionString {
									v2, b2 := attributes.Get(string(semconv.DBUserKey))
									var r string
									if b2 {
										r = "://" + v2.AsString() + "@"
									} else {
										r = "://"
									}

									connection_string = strings.Replace(connection_string, "://", r, 1)
								}

								keyValue.Value = extensionlinux.StringPtr(connection_string)
								layerDetail.ExtCodes = append(layerDetail.GetExtCodes(), keyValue)
								//layerDetail.addExtendedcodes(keyValue);
							} else if v, b := attributes.Get(string(semconv.DBMongoDBCollectionKey)); b { // moteur MongoDB
								v2, _ := attributes.Get(string(semconv.DBSystemKey))
								v3, _ := attributes.Get(string(semconv.NetPeerNameKey))
								v4, _ := attributes.Get(string(semconv.NetPeerPortKey))

								// `${attributes[SemanticAttributes.DB_SYSTEM]}://${attributes[SemanticAttributes.NET_HOST_NAME]}:${attributes[SemanticAttributes.NET_HOST_PORT]}/${attributes[SemanticAttributes.DB_MONGODB_COLLECTION]}`;
								connection_string := fmt.Sprintf("%v://%v:%v/%v", v2.Str(), v3.Str(), v4.Int(), v.Str())

								if This.Parent.SendUserConnectionString {
									v2, b2 := attributes.Get(string(semconv.DBNameKey))
									var r string
									if b2 && len(v2.AsString()) > 0 {
										r = "://" + v2.AsString() + "@"
									} else {
										r = "://"
									}

									connection_string = strings.Replace(connection_string, "://", r, 1)
								}

								keyValue := &pdata.KeyValue{}
								keyValue.Reset()
								keyValue.Key = extensionlinux.StringPtr("jdbc.url")
								keyValue.Value = extensionlinux.StringPtr(connection_string)
								layerDetail.ExtCodes = append(layerDetail.GetExtCodes(), keyValue)
								//layerDetail.addExtendedcodes(keyValue);
							} else
							// moteur DBB importés : Sqlite3...
							if v, b = attributes.Get(string(semconv.DBSystemKey)); b {
								switch strings.ToLower(v.AsString()) {
								case "sqlite":
									fallthrough
								case "sqlite3":
									v2, _ := attributes.Get(string(semconv.DBNameKey))
									connection_string := fmt.Sprintf("%v///%v", v.AsString(), v2.Str())
									if This.Parent.SendUserConnectionString {
										v3, b3 := attributes.Get(string(semconv.DBUserKey))
										var r string
										if b3 && len(v3.AsString()) > 0 {
											r = "://" + v3.AsString() + "@"
										} else {
											r = "://"
										}
										connection_string = strings.Replace(connection_string, "://", r, 1)
									}

									keyValue := &pdata.KeyValue{}
									keyValue.Reset()
									keyValue.Key = extensionlinux.StringPtr("jdbc.url")
									keyValue.Value = extensionlinux.StringPtr(connection_string)
									layerDetail.ExtCodes = append(layerDetail.GetExtCodes(), keyValue)
									//layerDetail.addExtendedcodes(keyValue);

								case "cassandra":
									v2, _ := attributes.Get(string(semconv.DBSystemKey))
									v3, _ := attributes.Get(string(semconv.NetPeerNameKey))
									v4, _ := attributes.Get(string(semconv.NetPeerPortKey))
									v5, _ := attributes.Get(string(semconv.DBNameKey))

									// `${attributes[SemanticAttributes.DB_SYSTEM]}://${attributes[SemanticAttributes.NET_PEER_NAME]}${attributes[SemanticAttributes.NET_PEER_PORT] != undefined ? ':':''}${attributes[SemanticAttributes.NET_PEER_PORT]}/${attributes[SemanticAttributes.DB_NAME]}`;
									connection_string := fmt.Sprintf("%v://%v:%v/%v", v2.Str(), v3.Str(), v4.Int(), v5.Str())
									if This.Parent.SendUserConnectionString {
										v3, b3 := attributes.Get(string(semconv.DBUserKey))
										var r string
										if b3 && len(v3.AsString()) > 0 {
											r = "://" + v3.AsString() + "@"
										} else {
											r = "://"
										}
										connection_string = strings.Replace(connection_string, "://", r, 1)
									}

									keyValue := &pdata.KeyValue{}
									keyValue.Reset()
									keyValue.Key = extensionlinux.StringPtr("jdbc.url")
									keyValue.Value = extensionlinux.StringPtr(connection_string)
									layerDetail.ExtCodes = append(layerDetail.GetExtCodes(), keyValue)
									//layerDetail.addExtendedcodes(keyValue);

								default:
									v2, _ := attributes.Get(string(semconv.DBSystemKey))
									v3, _ := attributes.Get(string(semconv.NetPeerNameKey))
									v4, _ := attributes.Get(string(semconv.NetPeerPortKey))
									v5, _ := attributes.Get(string(semconv.DBNameKey))

									// `${attributes[SemanticAttributes.DB_SYSTEM]}://${attributes[SemanticAttributes.NET_PEER_NAME]}${attributes[SemanticAttributes.NET_PEER_PORT] != undefined ? ':':''}${attributes[SemanticAttributes.NET_PEER_PORT]}/${attributes[SemanticAttributes.DB_NAME]}`;
									connection_string := fmt.Sprintf("%v://%v:%v/%v", v2.Str(), v3.Str(), v4.Str(), v5.Str())
									if This.Parent.SendUserConnectionString {
										v3, b3 := attributes.Get(string(semconv.DBUserKey))
										var r string
										if b3 && len(v3.AsString()) > 0 {
											r = "://" + v3.AsString() + "@"
										} else {
											r = "://"
										}
										connection_string = strings.Replace(connection_string, "://", r, 1)
									}

									keyValue := &pdata.KeyValue{}
									keyValue.Reset()
									keyValue.Key = extensionlinux.StringPtr("jdbc.url")
									keyValue.Value = extensionlinux.StringPtr(connection_string)
									layerDetail.ExtCodes = append(layerDetail.GetExtCodes(), keyValue)
									//layerDetail.addExtendedcodes(keyValue);
								}
							}

							InsertExtCode(string(semconv.DBNameKey), "jdbc.name", attributes, layerDetail)
							InsertExtCode(string(semconv.DBSystemKey), "jdbc.system", attributes, layerDetail)
							InsertExtCode(string(semconv.DBUserKey), "jdbc.product.user", attributes, layerDetail)
							/*
								SA := []string{string(semconv.NetPeerNameKey), string(semconv.NetPeerPortKey), string(semconv.NetHostNameKey),
									string(semconv.NetHostPortKey),
								}*/
							SAN := []string{string(semconv.DBUserKey), string(semconv.DBSystemKey), string(semconv.DBNameKey), string(semconv.DBMongoDBCollectionKey),
								string(semconv.DBConnectionStringKey), string(semconv.DBStatementKey), pdata.SemanticAttributesX["HTTP_STACKTRACE"], LAYER_TYPE_SQL,
							}
							attributes.Range(func(k string, attribute pcommon.Value) bool {
								if !slices.Contains(SAN, k) {
									InsertExtCode(k, k, attributes, layerDetail)
									//layerDetail.addExtendedcodes(keyValue);
								}
								return true
							})

							// layerDetail.setExtendedcodesList();
							// layerDetail.addExtendedcodes();
							// layerDetail.setExtcodesList();
							// layerDetail.setCorrelationidsList();
							// layerSQL.setCallsList(layerDetail);

						} // Fin du Switch INSTRUMENT
					} else {
					}

					layerSQL.Calls = append(layerSQL.GetCalls(), layerDetail)
					layerSQL.Count = extensionlinux.NumberPtr(layerSQL.GetCount() + layerDetail.GetCount())
					layerSQL.Time = extensionlinux.NumberPtr(layerSQL.GetTime() + layerDetail.GetTime())
					layerSQL.Max = extensionlinux.NumberPtr(slices.Max([]int64{layerSQL.GetMax(), layerDetail.GetTime()}))
					layerSQL.Min = extensionlinux.NumberPtr(slices.Min([]int64{layerSQL.GetMin(), layerDetail.GetTime()}))

					/*
						var sqlRequest = new proto.org.nudge.buffer.SqlRequest();

						if (typeof attributes[SemanticAttributes.DB_STATEMENT] != 'undefined')
						  sqlRequest.setSql(attributes[SemanticAttributes.DB_STATEMENT]);

						sqlRequest.setStarttime( getTimestamp(sps['startTime']) );
						sqlRequest.setEndtime( getTimestamp(sps['endTime']) );

						if (typeof attributes[SemanticAttributes.DB_CONNECTION_STRING] != 'undefined')
						  sqlRequest.setServerurl(attributes[SemanticAttributes.DB_CONNECTION_STRING]);

						// sqlRequest.setCount();
						// sqlRequest.setQueryavg();
						// sqlRequest.setQuerymin();
						// sqlRequest.setQuerymax();
						// sqlRequest.setServerurl();
						// sqlRequest.setFetchcount();
						// sqlRequest.setFetchavg();
						// sqlRequest.setFetchmin();
						// sqlRequest.setFetchmax();
						// sqlRequest.setUrlid();
						// sqlRequest.setRequuid();
						// sqlRequest.setRequesttype();
						// sqlRequest.setParametersList();

						this.transaction.addSqlrequests(sqlRequest);
					*/
				} // fin de if DB || GQL
			} // boucle 3
		} // boucle 2
	} // boucle 1

	/* Partie Profiling INUTILE ICI
	if (global.configurationAgentNodeJS?.Console?.logDetail2 === true)
	{
	  let functionPointEntreeTimeLineNode = getLast(functionPointEntreeTimeLineNodes);
	  let T = functionPointEntreeTimeLineNodes.reduce((a, e) => a + e.N, 0);
	  let s = '[' + functionPointEntreeTimeLineNodes.reduce((a, e, i) => {
		return a + `${e.N}(${e.M})[${functionPointEntreeTimeLineNode.I - e.I}]${i+1 < functionPointEntreeTimeLineNodes.length ? ',':''}`;
	  }, "") + ']';
	  MyConsole.log(`going to flush rawdata -> global.inspector.profile.cmpt / functionPointEntreeTimeLineNodes -> ${global.inspector.profile.cmpt} / ${T} ${s}} : (${global.inspector.profile.cmpt == T ? "OK" : "KO"})`);
	}

	global.inspector.profile.cmpt = 0
	*/

} // FIn de la fonction

func InsertExtCode(key string, keyX string, attr pcommon.Map, layerDetail *pdata.LayerDetail) (*pdata.KeyValue, bool) {
	var keyValue *pdata.KeyValue
	v, b := attr.Get(key)
	if b {
		keyValue, _ = extensionlinux.Find(layerDetail.GetExtCodes(), func(e *pdata.KeyValue) bool {
			return e.GetKey() == keyX
		})
		if keyValue != nil {
			//keyValue.Key = extensionlinux.StringPtr(keyX)
			keyValue.Value = extensionlinux.StringPtr(v.AsString())
		} else {
			keyValue = &pdata.KeyValue{}
			keyValue.Reset()
			keyValue.Key = extensionlinux.StringPtr(keyX)
			keyValue.Value = extensionlinux.StringPtr(v.AsString())
			layerDetail.ExtCodes = append(layerDetail.GetExtCodes(), keyValue)
		}
	}

	return keyValue, b
}

func (transaction *TransactionNudge) InsertExtendedCode(key string, keyX string, attr pcommon.Map) (*pdata.KeyValue, bool) {
	var keyValue *pdata.KeyValue
	v, b := attr.Get(key)
	if b {
		keyValue, _ = extensionlinux.Find(transaction.GetExtendedCodes(), func(e *pdata.KeyValue) bool {
			return e.GetKey() == keyX
		})
		if keyValue != nil {
			//keyValue.Key = extensionlinux.StringPtr(keyX)
			keyValue.Value = extensionlinux.StringPtr(v.AsString())
		} else {
			keyValue = &pdata.KeyValue{}
			keyValue.Reset()
			keyValue.Key = extensionlinux.StringPtr(keyX)
			keyValue.Value = extensionlinux.StringPtr(v.AsString())
			transaction.ExtendedCodes = append(transaction.GetExtendedCodes(), keyValue)
		}
	}

	return keyValue, b
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

func (This *NudgeExporter) MetricsExportRequest2Rawdata(tr *pmetricnudge.ExportRequest) {
	//metrics := tr.Metrics()
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

func (This *NudgeExporter) LogsExportRequest2Rawdata(tr *plognudge.ExportRequest) {
	//metrics := tr.Metrics()
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

func (This *NudgeExporter) ProfilesExportRequest2Rawdata(tr *pprofilenudge.ExportRequest) {

}

func (e *baseExporter) export(ctx context.Context, url string, request []byte, partialSuccessHandler partialSuccessHandler) error {
	// Met à nil le pointeur car flush des rawdata
	e.nudgeExporter.Rawdata = nil

	e.logger.Debug("Preparing to make HTTP request", zap.String("url", url))

	br := bytes.NewReader(request)
	req, err := http.NewRequestWithContext(ctx, e.config.NudgeHTTPClientConfig.Method, url, br)
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

	if e.config.NudgeHTTPClientConfig.Console.Verbosity.String() == "Detailed" {
		e.logger.Info("Send http", zap.String("url", req.Method+" "+req.URL.Scheme+"://"+req.URL.Host+":"+req.URL.Port()+req.URL.Path), zap.Int64("ContentLength", req.ContentLength), zap.String("StatusCode", resp.Status))
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

type Server struct {
	Name       string
	Type       string
	Nom        string
	Instrument string
}

func newServer() *Server {
	return &Server{}
}

type TransactionNudge_TAG struct {
	Correlationid uint32
	Urlid         uint32
	Serveur       *Server
	Code          string
	TraceId       string
	ApplicationID string
	Language      string
	UrlAppId      *string
}

type TransactionNudge struct {
	*pdata.Transaction
	ErrorsX []*ErrorX
	TAG     *TransactionNudge_TAG
}

func newTransactionNudge() *TransactionNudge {
	t := &TransactionNudge{
		Transaction: &pdata.Transaction{},
		TAG: &TransactionNudge_TAG{
			Correlationid: uint32(0),
			Urlid:         uint32(0),
			Serveur:       nil,
		},
	}

	t.Transaction.Reset()

	return t
}
func (This *TransactionNudge) GetErrors() []*ErrorX {
	if This != nil {
		return This.ErrorsX
	}
	return nil
}

type NudgeExporter struct {
	Rawdata        *pdata.RawData
	ThreadInfoList *pdata.ThreadInfoList
	Cmpt           int64
	AllDrives      map[string]any
	Agentiduuid    string
	IndexRawdata   int
	Components     []*pdata.Component
	Hostname       string

	Parent       *Config
	Transactions []*TransactionNudge
	Transaction  *TransactionNudge
	Dns          string
}

func newNudgeExporter(parent *Config) *NudgeExporter {
	This := &NudgeExporter{}
	This.Rawdata = nil
	This.Cmpt = 0
	This.AllDrives = map[string]interface{}{}
	This.Agentiduuid = GetUuid("CollecteurOTLP").String()
	This.Components = make([]*pdata.Component, 0)
	This.Hostname, _ = os.Hostname()
	This.Parent = parent
	This.Transactions = make([]*TransactionNudge, 0)
	This.Transaction = newTransactionNudge()

	This.InitDrives()

	// Recuperer
	ips, err := net.LookupIP("google.com")
	if err != nil {
		//fmt.Println("Erreur:", err)
	} else {
		ipsX := extensionlinux.Map(ips, func(ip net.IP) string {
			return ip.String()
		})

		This.Dns = strings.Join(ipsX, ";")
	}

	return This
}

func (This *NudgeExporter) MarshalJSON() ([]byte, error) {
	jsonData, err := json.Marshal(This.Rawdata)
	return jsonData, err
}

func (This *NudgeExporter) MarshalProto() ([]byte, error) {
	// Buffer pour stocker les données binaires
	//var buf bytes.Buffer
	// Création d'un encodeur GOB
	//enc := gob.NewEncoder(&buf)
	// Encodage de la structure en binaire
	//err := enc.Encode(This.Rawdata)

	out, err := proto.Marshal(This.Rawdata)
	return out, err
}

func (This *NudgeExporter) Attributes2URL(cfg component.Config, attributes pcommon.Map) *string {
	v, b := attributes.Get(Nudge_application_id)
	if b {
		oCfg := cfg.(*Config)
		s, err := composeSignalURL(oCfg, oCfg.TracesEndpoint, oCfg.NudgeHTTPClientConfig.PathCollect, v.AsString()) //"traces", "v1")
		if err != nil {
			return nil
		}

		return &s
	}

	return nil
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

/**
* Urilid = 0 supprime du tableau des transactions et envoie dans le rawdata
* Urilid = 2 supprime du tableau des transactions mais n'envoie pas dans le rawdata
* Urilid = 1 garde dans le tableau des transactions jusqu'au timeout puis envoie dans le rawdata
 */
func (This *NudgeExporter) SendTransactions(app_id string) ([]string, *string) {
	m := len(This.Rawdata.Transactions)
	n := 0
	k := len(This.Transactions)
	languages := make([]string, 0)
	var urlAppId *string = nil
	for i := 0; i < len(This.Transactions); i++ {
		t := This.Transactions[i]

		if app_id == "" || t.TAG.ApplicationID == app_id {

			for _, e := range t.GetErrors() {
				p := e.ToError()
				t.Errors = append(t.Errors, p)
			}

			if slices.Contains([]uint32{0, 2}, t.TAG.Urlid) {
				if slices.Contains([]uint32{0}, t.TAG.Urlid) {
					This.Rawdata.Transactions = append(This.Rawdata.Transactions, t.Transaction)
					languages = extensionlinux.AppendUnique(languages, t.TAG.Language)
					if app_id != "" && urlAppId == nil {
						urlAppId = t.TAG.UrlAppId
					}
				}

				This.Transactions = extensionlinux.Splice(This.Transactions, i, 1) //.splice(i, 1);
				i--
			} else if slices.Contains([]uint32{1}, t.TAG.Urlid) {
				st := t.GetStartTime()
				st = (time.Now().UnixMilli() - st) / 1000
				if st > 0 && st > This.Parent.TimeoutBufferTransaction {
					if This.Parent.Console.Verbosity.String() == "Detailed" {
						fmt.Println(fmt.Printf("Transaction non terminée(s), TIMEOUT %vs , language:%v , app_id:%v , %v", This.Parent.TimeoutBufferTransaction, t.TAG.Language, t.TAG.ApplicationID, t.GetCode()))
					}

					This.Rawdata.Transactions = append(This.Rawdata.Transactions, t.Transaction)
					languages = extensionlinux.AppendUnique(languages, t.TAG.Language)
					if app_id != "" && urlAppId == nil {
						urlAppId = t.TAG.UrlAppId
					}
					This.Transactions = extensionlinux.Splice(This.Transactions, i, 1)
					i--
					n++
				}
			}
		}
	}

	if This.Parent.Console.Verbosity.String() == "Detailed" {
		fmt.Println(fmt.Sprintf("Ajout de %v/%v transactions, Nombre de transaction(s) non terminée(s) : %v", len(This.Rawdata.Transactions)-m, k, n))
	}

	This.Transaction = nil

	return languages, urlAppId
}

func BuildPathCollect(dateStr string, filenameN int) string {
	// "./collectes/collecte_2025-04-10_file123.dat"
	relativePath := fmt.Sprintf("./collectes/collecte_%v_%v.dat", dateStr, filenameN)

	// Résout le chemin absolu
	absPath, err := filepath.Abs(relativePath)
	if err != nil {
		// En cas d'erreur, retourne le chemin relatif
		return relativePath
	}

	return absPath
}

func (This *NudgeExporter) FlushRawdataToFileDisk(request []byte) {
	dateStr := "X" // `${String(d.getFullYear()).padStart(4,'0')}_${String(d.getMonth()+1).padStart(2,'0')}_${String(d.getDate()).padStart(2,'0')}__${String(d.getHours()).padStart(2,'0')}_${String(d.getMinutes()).padStart(2,'0')}_${String(d.getSeconds()).padStart(2,'0')}`;
	//filename := path.resolve(__dirname, `./collectes/collecte_${dateStr}_${filename_N}.dat`);

	filename := BuildPathCollect(dateStr, filename_N)
	err := os.WriteFile(filename, request, 0644)
	if err != nil {
		fmt.Println(fmt.Sprintf("Erreur lors de l'écriture du fichier : %v : %v", filename, err))
	} else {
		fmt.Println(fmt.Sprintf("Rawdata envoyé dans le fichier : %v", filename))
	}
	filename_N++
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
		This.Rawdata.Reset()

		This.Rawdata.Id = &This.Cmpt // setId(This.Cmpt++)
		This.Cmpt++
		This.Rawdata.AgentId = &This.Agentiduuid // .setAgentid(This.Agentiduuid)

		if !This.Parent.InsertResource {
			return
		}

		This.Rawdata.ServerConfig = &pdata.ServerConfig{} // clearServerConfig()
		This.Rawdata.ServerConfig.Reset()
		This.ThreadInfoList = &pdata.ThreadInfoList{}
		This.ThreadInfoList.Reset()
		This.Rawdata.ThreadInfos = This.ThreadInfoList.GetThreads() // clearThreadinfosList()

		This.Rawdata.SystemMetrics = make([]*pdata.SystemMetricSample, 0) // clearSystemmetricsList()
		This.Rawdata.ThreadActivity = &pdata.ThreadActivity{}             // .clearThreadactivity()
		This.Rawdata.ThreadActivity.Reset()
		classDictionary := &pdata.Dictionary{} // new proto.org.nudge.buffer.Dictionary();
		classDictionary.Reset()
		methodDictionary := &pdata.Dictionary{} // new proto.org.nudge.buffer.Dictionary();
		methodDictionary.Reset()
		This.Rawdata.MethodDictionary = methodDictionary // .setMethoddictionary(methodDictionary);
		This.Rawdata.ClassDictionary = classDictionary   // .setClassdictionary(classDictionary);

		This.Rawdata.Hostname = extensionlinux.StringPtr(service_hostname) // .setHostname(service_hostname)

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

		This.Rawdata.Hostkey = extensionlinux.NumberPtr(hostKey) // .setHostkey(hostkey);
		//This.Rawdata.setSegmentdictionary();
		//This.Rawdata.setQuerydictionary();
		//This.Rawdata.setClassdictionary();
		//This.Rawdata.setMethoddictionary();
		//This.Rawdata.setUseragent("TOTO");
		//This.Rawdata.setMbeandictionary();
		//This.Rawdata.setJmsdictionary();

		//var o = This.Rawdata.toObject();

		if indexRawdata_MIN == -1 || This.IndexRawdata < indexRawdata_MIN {
			serverConfig := &pdata.ServerConfig{} // new proto.org.nudge.buffer.ServerConfig();
			serverConfig.Reset()
			serverConfig.OsArch = extensionlinux.StringPtr(runtime.GOARCH)             // .setOsarch(os.arch());
			serverConfig.OsName = extensionlinux.StringPtr(runtime.GOOS)               // .setOsname(os.version());
			serverConfig.OsVersion = extensionlinux.StringPtr(mySystem.GetOSVersion()) // .setOsversion(os.release());

			//cpus := cpu // os.cpus();
			cpus, _ := cpu.Counts(true)
			serverConfig.AvailableProcessors = extensionlinux.NumberPtr(int32(cpus))         // .setAvailableprocessors(cpus.length);
			serverConfig.VmName = extensionlinux.StringPtr("CollectorOTLP@" + This.Hostname) // .setVmname(`CollectorOTLP@${os.hostname}`); //  pjson.description;
			//TEMP serverConfig.VmVendor = pjson.vendor                       // .setVmvendor(pjson.vendor);
			infos, _ := cpu.Info()
			//versions := Object.keys(process.versions).map(e => `${e}@${process.versions[e]}`).join(';');
			versions := extensionlinux.Map(infos, func(t cpu.InfoStat) string {
				return t.ModelName + "|" + t.Family + "|" + strconv.FormatFloat(t.Mhz, 'f', 0, 64)
			})
			serverConfig.VmVersion = extensionlinux.StringPtr(strings.Join(versions, ";")) // .setVmversion(versions);   // pjson.version);
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

			serverConfig.StartTime = extensionlinux.NumberPtr(time.Now().UnixMilli()) // .setStarttime(Date.now()); // PLantage lors du serialize Rawdata

			// Simule __dirname en JavaScript
			// Compatible Windows & Linux, car filepath.Join utilise le bon séparateur (\ pour Windows, / pour Linux/Mac).
			exePath, err := os.Executable()
			if err != nil {
				dir := filepath.Dir(exePath) // __dirname en JS
				// Construction du chemin absolu
				absolutePath := filepath.Join(dir, "")                              // appJS est le chemin absolue de l'application cliente
				serverConfig.BootClassPath = extensionlinux.StringPtr(absolutePath) // .setBootclasspath(path.resolve(__dirname, appJS));
			}
			// LINUX ADAPT
			if len(This.Dns) > 0 {
				ip, err := gateway.DiscoverGateway()
				if err == nil {
					a, err := net.LookupCNAME(ip.String())
					if err == nil {
						serverConfig.CanonicalHostName = extensionlinux.StringPtr(a)
					}
				}
			}

			//serverConfig.setCanonicalhostname(canonHost.name);
			//serverConfig.setHostaddress();
			serverConfig.HostName = extensionlinux.StringPtr(service_hostname)              // .setHostname(service_hostname);
			serverConfig.AppName = extensionlinux.StringPtr(This.Parent.ApplicationName)    // .setAppname(configurationNudge.Section("NodeJS").Key("applicationName").String());
			platform := runtime.GOOS                                                        // .platform();
			serverConfig.Environment = extensionlinux.StringPtr(platform)                   // .setEnvironment(platform.toString());
			serverConfig.ServerName = extensionlinux.StringPtr(This.Parent.ApplicationName) // .setServername(configurationNudge.Section("NodeJS").Key("applicationName").String());
			//TEMP serverConfig.ServerInfo = pjson.description                                                               // .setServerinfo(pjson.description);
			//TEMP serverConfig.ServerPort = process.debugPort                                                               // .setServerport(process.debugPort);
			serverConfig.InputArguments = extensionlinux.StringPtr(strings.Join(os.Args, ";")) // .setInputarguments(process.argv.join(';'));
			//serverConfig.setDiagnosticconfig();
			//TEMP serverConfig.NudgeVersion = pjson.version // .setNudgeversion(pjson.version);

			jvmInfo := &pdata.JvmInfo{} // new proto.org.nudge.buffer.JvmInfo();
			jvmInfo.Reset()
			jvmInfo.HostName = &This.Hostname // .setHostname(service_hostname);
			kvs := make([]*pdata.KeyValue, 0) // new Array();

			for _, k := range os.Environ() {
				k = strings.SplitN(k, "=", 2)[0]
				v := os.Getenv(k)
				kv := &pdata.KeyValue{} // new proto.org.nudge.buffer.KeyValue();
				kv.Reset()
				kv.Key = extensionlinux.StringPtr(k)   // .setKey(k);
				kv.Value = extensionlinux.StringPtr(v) // .setValue(v);
				kvs = append(kvs, kv)
			}
			jvmInfo.SystemProperties = kvs // .setSystempropertiesList(kvs);
			serverConfig.JvmInfo = jvmInfo // .setJvminfo(jvmInfo);

			This.Rawdata.ServerConfig = serverConfig // .setServerconfig(serverConfig);
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

		memoryUsage, memStats, _ := mySystem.GetMemoryWindows()

		getAlldrivesX := func() {

			systemMetricSample := &pdata.SystemMetricSample{} // new proto.org.nudge.buffer.SystemMetricSample();
			systemMetricSample.Reset()
			dateNow := time.Now().UTC().UnixMilli()                                                  // Date.now();
			systemMetricSample.Key = extensionlinux.StringPtr("mem.free")                            // .setKey(`mem.free`);
			systemMetricSample.Timestamp = &dateNow                                                  // .setTimestamp(dateNow);
			systemMetricSample.LongValue = extensionlinux.NumberPtr(int64(memoryUsage.AvailPhys))    // .setLongvalue(os.freemem());
			This.Rawdata.SystemMetrics = append(This.Rawdata.GetSystemMetrics(), systemMetricSample) // .addSystemmetrics(systemMetricSample);

			systemMetricSample = &pdata.SystemMetricSample{} // new proto.org.nudge.buffer.SystemMetricSample();
			systemMetricSample.Reset()
			systemMetricSample.Key = extensionlinux.StringPtr("mem.total")                           // .setKey(`mem.total`);
			systemMetricSample.Timestamp = &dateNow                                                  // .setTimestamp(dateNow);
			systemMetricSample.LongValue = extensionlinux.NumberPtr(int64(memoryUsage.TotalPhys))    // .setLongvalue(os.totalmem());
			This.Rawdata.SystemMetrics = append(This.Rawdata.GetSystemMetrics(), systemMetricSample) // .addSystemmetrics(systemMetricSample);

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
				systemMetricSample.Reset()
				systemMetricSample.Key = extensionlinux.StringPtr("cpu.load") // .setKey(`mem.total`);
				//TEMP systemMetricSample.Timestamp = oSU.dateNow                                          // .setTimestamp(dateNow);
				//TEMP systemMetricSample.DoubleValue = oSU.cpuPercentage / 100                            // .setLongvalue(os.totalmem());
				//This.Rawdata.SystemMetrics = append(This.Rawdata.GetSystemMetrics(), systemMetricSample) // .addSystemmetrics(systemMetricSample);

				systemMetricSample = &pdata.SystemMetricSample{}
				systemMetricSample.Reset()
				systemMetricSample.Key = extensionlinux.StringPtr("load")
				//TEMP systemMetricSample.Timestamp = oSU.dateNow
				//TEMP systemMetricSample.DoubleValue = oSU.cpuPercentageNode
				//This.Rawdata.SystemMetrics = append(This.Rawdata.GetSystemMetrics(), systemMetricSample) // .addSystemmetrics(systemMetricSample);
			}

			// La swap ne fonctionne pas sous NodeJS
			// swap.free => external
			// swap.total => rss
			//memoryUsage = process.memoryUsage();
			if memoryUsage.AvailPhys > 0 { //  || global.gc)
				systemMetricSample = &pdata.SystemMetricSample{}
				systemMetricSample.Reset()
				systemMetricSample.Key = extensionlinux.StringPtr("swap.free")
				systemMetricSample.Timestamp = &dateNow
				systemMetricSample.LongValue = extensionlinux.NumberPtr(int64(memoryUsage.AvailPhys))
				This.Rawdata.SystemMetrics = append(This.Rawdata.GetSystemMetrics(), systemMetricSample) // .addSystemmetrics(systemMetricSample);
			}
			if memoryUsage.TotalPhys > 0 { //  || global.gc)
				systemMetricSample = &pdata.SystemMetricSample{}
				systemMetricSample.Reset()
				systemMetricSample.Key = extensionlinux.StringPtr("swap.total")
				systemMetricSample.Timestamp = &dateNow
				systemMetricSample.LongValue = extensionlinux.NumberPtr(int64(memoryUsage.TotalPhys))
				This.Rawdata.SystemMetrics = append(This.Rawdata.GetSystemMetrics(), systemMetricSample) // .addSystemmetrics(systemMetricSample);
			}

			//TEMP if oSU.openFd != "not supported" {
			systemMetricSample = &pdata.SystemMetricSample{}
			systemMetricSample.Reset()
			systemMetricSample.Key = extensionlinux.StringPtr("proc.fd.open")
			systemMetricSample.Timestamp = &dateNow
			//TEMP systemMetricSample.LongValue = oSU.openFd
			//This.Rawdata.SystemMetrics = append(This.Rawdata.GetSystemMetrics(), systemMetricSample) // .addSystemmetrics(systemMetricSample);
			//TEMP }

			//let swap = getSwapSize();
			// Linux stat.Totalswap * uint64(stat.Unit), stat.Freeswap * uint64(stat.Unit)
			// Windows
			systemMetricSample = &pdata.SystemMetricSample{}
			systemMetricSample.Reset()
			systemMetricSample.Key = extensionlinux.StringPtr("disk.total#[SWAP]")
			systemMetricSample.Timestamp = &dateNow
			//systemMetricSample.setDoublevalue();
			systemMetricSample.LongValue = extensionlinux.NumberPtr(int64(memoryUsage.TotalPageFile))
			This.Rawdata.SystemMetrics = append(This.Rawdata.GetSystemMetrics(), systemMetricSample) // .addSystemmetrics(systemMetricSample);

			systemMetricSample = &pdata.SystemMetricSample{}
			systemMetricSample.Reset()
			systemMetricSample.Key = extensionlinux.StringPtr("disk.free#[SWAP]")
			systemMetricSample.Timestamp = &dateNow
			//systemMetricSample.setDoublevalue();
			systemMetricSample.LongValue = extensionlinux.NumberPtr(int64(memoryUsage.AvailPageFile))
			This.Rawdata.SystemMetrics = append(This.Rawdata.GetSystemMetrics(), systemMetricSample) // .addSystemmetrics(systemMetricSample);

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
						systemMetricSample.Reset()
						systemMetricSample.Key = &key
						systemMetricSample.Timestamp = extensionlinux.NumberPtr(dateNow)
						//systemMetricSample.setDoublevalue();
						//TEMP systemMetricSample.LongValue = drive.size
						//This.Rawdata.SystemMetrics = append(This.Rawdata.GetSystemMetrics(), systemMetricSample) // .addSystemmetrics(systemMetricSample);

						if drives, b := drive["mountpoints"].([]any); b {
							for _, driveX := range drives {
								if part, b := driveX.(map[string]any); b {
									var diskSpace any
									if t, b := part["path"].(string); b && t != "[SWAP]" {
										//TEMP diskSpace = This.AllDrives["drives"][part.path]
									} else {
										diskSpace = mySystem.TotalFree{Total: 0 /*swap.size*/, Free: 0 /*wap.free*/}
									}

									if diskSpace != nil {
										systemMetricSample = &pdata.SystemMetricSample{}
										systemMetricSample.Reset()
										systemMetricSample.Key = extensionlinux.StringPtr("disk.total#${part.path}")
										systemMetricSample.Timestamp = extensionlinux.NumberPtr(dateNow)
										//systemMetricSample.setDoublevalue();
										//TEMP systemMetricSample.LongValue = extensionlinux.NumberPtr(int64(diskSpace.size))

										This.Rawdata.SystemMetrics = append(This.Rawdata.GetSystemMetrics(), systemMetricSample) // .addSystemmetrics(systemMetricSample);

										systemMetricSample = &pdata.SystemMetricSample{}
										systemMetricSample.Reset()
										systemMetricSample.Key = extensionlinux.StringPtr("disk.free#${part.path}")
										systemMetricSample.Timestamp = extensionlinux.NumberPtr(dateNow)
										//systemMetricSample.setDoublevalue();
										//TEMP systemMetricSample.LongValue = extensionlinux.NumberPtr(int64(diskSpace.free))

										This.Rawdata.SystemMetrics = append(This.Rawdata.GetSystemMetrics(), systemMetricSample) // .addSystemmetrics(systemMetricSample);
									}
								}
							}
						}
					}
				}
			}
		}
		getAlldrivesX()

		heapMemory := &pdata.HeapMemory{} // new proto.org.nudge.buffer.HeapMemory();
		heapMemory.Reset()
		heapMemory.StartTime = extensionlinux.NumberPtr(time.Now().UTC().UnixMilli())
		heapMemory.Used = extensionlinux.NumberPtr(int64(memStats.HeapInuse))
		heapMemory.Total = extensionlinux.NumberPtr(int64(memStats.TotalAlloc))
		heapMemory.EndTime = extensionlinux.NumberPtr(time.Now().UTC().UnixMilli())
		This.Rawdata.HeapMemory = heapMemory

		threadActivity := &pdata.ThreadActivity{} // new proto.org.nudge.buffer.ThreadActivity();
		threadActivity.Reset()
		threadActivity.StartTime = extensionlinux.NumberPtr(time.Now().UTC().UnixMilli()) // process.startTime
		threadActivity.Count = extensionlinux.NumberPtr[int32](1)
		threadActivity.DaemonThreadCount = extensionlinux.NumberPtr[int32](0)
		threadActivity.NewThreadCount = extensionlinux.NumberPtr[int32](0)
		//cpuUsage := process.cpuUsage()
		/*
					    // Utilisation CPU globale sur 1 seconde
			    percentages, err := cpu.Percent(time.Second, false)
			    if err != nil {
			        panic(err)
			    }
			    fmt.Printf("Utilisation CPU globale : %.2f%%\n", percentages[0])

			    // Utilisation CPU par core
			    perCore, err := cpu.Percent(time.Second, true)
			    if err != nil {
			        panic(err)
			    }
			    for i, pct := range perCore {
			        fmt.Printf("Core %d : %.2f%%\n", i, pct)
			    }
		*/

		/*
				// Snapshot 1
			t1, err := cpu.Times(false)
			if err != nil {
				panic(err)
			}
			time.Sleep(1 * time.Second) // délai pour mesurer la différence

			// Snapshot 2
			t2, err := cpu.Times(false)
			if err != nil {
				panic(err)
			}

			// Calcul des deltas
			deltaUser := t2[0].User - t1[0].User
			deltaSystem := t2[0].System - t1[0].System
			deltaIdle := t2[0].Idle - t1[0].Idle
			deltaTotal := deltaUser + deltaSystem + deltaIdle + (t2[0].Nice - t1[0].Nice) + (t2[0].Iowait - t1[0].Iowait) + (t2[0].Irq - t1[0].Irq) + (t2[0].Softirq - t1[0].Softirq) + (t2[0].Steal - t1[0].Steal)

			userPercent := (deltaUser / deltaTotal) * 100
			systemPercent := (deltaSystem / deltaTotal) * 100
			idlePercent := (deltaIdle / deltaTotal) * 100

			fmt.Printf("User CPU :   %.2f%%\n", userPercent)
			fmt.Printf("System CPU : %.2f%%\n", systemPercent)
			fmt.Printf("Idle CPU :   %.2f%%\n", idlePercent)
		*/
		threadActivity.CpuTime = extensionlinux.NumberPtr(int64(metricsInterne.CPUUsage.Value))
		threadActivity.UserTime = extensionlinux.NumberPtr(int64(metricsInterne.CPUUsage.Value))
		threadActivity.EndTime = extensionlinux.NumberPtr(threadActivity.GetStartTime() + int64(time.Second*5)) //extensionlinux.NumberPtr(time.Now().UTC().UnixMilli())
		/*
					infos, _ := cpu.Info()
			for _, ci := range infos {
			    fmt.Printf("CPU : %v @ %.2f MHz (Cores logiques: %d)\n", ci.ModelName, ci.Mhz, ci.Cores)
			}
		*/
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
