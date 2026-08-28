package isolated

import (
	"fmt"
	"net/http"
	"time"

	"github.com/Grove-Computing/Growse/internal/dom"
	"github.com/Grove-Computing/Growse/internal/events"
	"github.com/Grove-Computing/Growse/internal/network"
	runtimemodel "github.com/Grove-Computing/Growse/internal/runtime"
	storagecore "github.com/Grove-Computing/Growse/internal/storage"
)

type sandboxStatusResponse struct {
	runtimemodel.SandboxStatus
	ProfileVersion  int `json:"profileVersion"`
	ProtocolVersion int `json:"protocolVersion"`
	WorkerPID       int `json:"workerPid"`
}

type wireScript struct {
	Engine        runtimemodel.Engine         `json:"engine"`
	Kind          runtimemodel.ScriptKind     `json:"kind,omitempty"`
	SourceURL     string                      `json:"sourceUrl,omitempty"`
	Source        string                      `json:"source"`
	Inline        bool                        `json:"inline,omitempty"`
	Integrity     string                      `json:"integrity,omitempty"`
	CrossOrigin   string                      `json:"crossOrigin,omitempty"`
	Schedule      runtimemodel.ScriptSchedule `json:"schedule,omitempty"`
	DocumentOrder int                         `json:"documentOrder,omitempty"`
	FetchOrder    int                         `json:"fetchOrder,omitempty"`
}

type loadRequest struct {
	Engine          runtimemodel.Engine           `json:"engine"`
	Scripts         []wireScript                  `json:"scripts"`
	Document        dom.DocumentSnapshot          `json:"document"`
	BaseURL         string                        `json:"baseUrl"`
	ResourceBaseURL string                        `json:"resourceBaseUrl,omitempty"`
	ImportMap       map[string]string             `json:"importMap,omitempty"`
	LocalStorage    []storagecore.Entry           `json:"localStorage,omitempty"`
	SessionStorage  []storagecore.Entry           `json:"sessionStorage,omitempty"`
	StorageSource   storagecore.MutationSource    `json:"storageSource"`
	HistoryLength   int                           `json:"historyLength"`
	HistoryState    string                        `json:"historyState,omitempty"`
	Frames          []wireFrame                   `json:"frames,omitempty"`
	FramePolicy     runtimemodel.FramePolicy      `json:"framePolicy,omitempty"`
	Window          runtimemodel.WindowContext    `json:"window"`
	ServiceWorker   bool                          `json:"serviceWorker,omitempty"`
	Media           runtimemodel.MediaEnvironment `json:"media"`
}

type renderReadRequest struct {
	NodeID dom.NodeID `json:"nodeId"`
}

type renderReadResponse struct {
	Snapshot runtimemodel.RenderSnapshot `json:"snapshot"`
}

type mediaEnvironmentEvent struct {
	Media runtimemodel.MediaEnvironment `json:"media"`
}

type wireFrame struct {
	ID         uint64                `json:"id"`
	ElementID  dom.NodeID            `json:"elementId"`
	Generation uint64                `json:"generation"`
	Origin     string                `json:"origin"`
	URL        string                `json:"url,omitempty"`
	SameOrigin bool                  `json:"sameOrigin"`
	Document   *dom.DocumentSnapshot `json:"document,omitempty"`
}

type frameMutationRequest struct {
	ID         uint64               `json:"id"`
	Generation uint64               `json:"generation"`
	Document   dom.DocumentSnapshot `json:"document"`
}

type postMessageRequest struct {
	Target       runtimemodel.WindowReference `json:"target"`
	TargetOrigin string                       `json:"targetOrigin"`
	Payload      []byte                       `json:"payload"`
}

type messageRequest struct {
	Event runtimemodel.MessageEvent `json:"event"`
}

type serviceWorkerRegisterRequest struct {
	ScriptURL string `json:"scriptUrl"`
	Scope     string `json:"scope,omitempty"`
}

type serviceWorkerScopeRequest struct {
	Scope string `json:"scope"`
}

type serviceWorkerClientRequest struct {
	URL string `json:"url,omitempty"`
}

type serviceWorkerRegistrationResponse struct {
	Found        bool                                   `json:"found"`
	Registration runtimemodel.ServiceWorkerRegistration `json:"registration"`
}

type serviceWorkerRegistrationsResponse struct {
	Registrations []runtimemodel.ServiceWorkerRegistration `json:"registrations"`
}

type mutationEvent struct {
	Document dom.DocumentSnapshot `json:"document"`
}

type consoleEvent struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

type frameRequest struct {
	UnixNano int64                `json:"unixNano"`
	Document dom.DocumentSnapshot `json:"document"`
}

type eventRequest struct {
	Document   dom.DocumentSnapshot `json:"document"`
	Type       events.Type          `json:"type"`
	Target     dom.NodeID           `json:"target"`
	X          float32              `json:"x,omitempty"`
	Y          float32              `json:"y,omitempty"`
	Value      string               `json:"value,omitempty"`
	Cancelable bool                 `json:"cancelable,omitempty"`
}

type eventResponse struct {
	Handled          bool `json:"handled"`
	DefaultPrevented bool `json:"defaultPrevented"`
}

type fetchRequest struct {
	Method      string                  `json:"method"`
	URL         string                  `json:"url"`
	Header      http.Header             `json:"header,omitempty"`
	Body        []byte                  `json:"body,omitempty"`
	SiteURL     string                  `json:"siteUrl,omitempty"`
	Kind        network.RequestKind     `json:"kind"`
	Engine      string                  `json:"engine,omitempty"`
	Credentials network.CredentialsMode `json:"credentials,omitempty"`
	CORS        bool                    `json:"cors,omitempty"`
}

type fetchResponse struct {
	URL         string      `json:"url"`
	StatusCode  int         `json:"statusCode"`
	Status      string      `json:"status"`
	Header      http.Header `json:"header,omitempty"`
	ContentType string      `json:"contentType,omitempty"`
	Body        []byte      `json:"body,omitempty"`
	Redirected  bool        `json:"redirected,omitempty"`
	CacheStatus string      `json:"cacheStatus,omitempty"`
}

type navigationRequest struct {
	URL string `json:"url"`
}

type historyStateRequest struct {
	State string `json:"state"`
	URL   string `json:"url"`
}

type historyTraverseRequest struct {
	Delta int `json:"delta"`
}

type historyInfoResponse struct {
	Length int    `json:"length"`
	State  string `json:"state,omitempty"`
}

type storageChangeEvent struct {
	Area   string             `json:"area"`
	Change storagecore.Change `json:"change"`
}

type storageExternalEvent struct {
	Area   string             `json:"area"`
	Change storagecore.Change `json:"change"`
}

type locationEvent struct {
	URL string `json:"url"`
}

type popStateEvent struct {
	State string `json:"state"`
}

type hashChangeEvent struct {
	OldURL string `json:"oldUrl"`
	NewURL string `json:"newUrl"`
}

type backgroundEvent struct {
	Background bool `json:"background"`
}

type boolResponse struct {
	Value bool `json:"value"`
}

func frameTime(request frameRequest) time.Time {
	return time.Unix(0, request.UnixNano)
}

func frameAccessToWire(frames []runtimemodel.FrameAccess) []wireFrame {
	result := make([]wireFrame, 0, len(frames))
	for _, frame := range frames {
		item := wireFrame{
			ID: frame.ID, ElementID: frame.ElementID, Generation: frame.Generation,
			Origin: frame.Origin, URL: frame.URL, SameOrigin: frame.SameOrigin,
		}
		if frame.SameOrigin && frame.Document != nil {
			snapshot := frame.Document.Snapshot()
			item.Document = &snapshot
		}
		result = append(result, item)
	}
	return result
}

func wireToFrameAccess(frames []wireFrame) ([]runtimemodel.FrameAccess, error) {
	result := make([]runtimemodel.FrameAccess, 0, len(frames))
	for _, frame := range frames {
		item := runtimemodel.FrameAccess{
			ID: frame.ID, ElementID: frame.ElementID, Generation: frame.Generation,
			Origin: frame.Origin, URL: frame.URL, SameOrigin: frame.SameOrigin,
		}
		if frame.SameOrigin {
			if frame.Document == nil {
				return nil, fmt.Errorf("same-origin Frame %d has no Document", frame.ID)
			}
			document, err := dom.NewDocumentFromSnapshot(*frame.Document)
			if err != nil {
				return nil, fmt.Errorf("decode Frame %d Document: %w", frame.ID, err)
			}
			item.Document = document
		}
		result = append(result, item)
	}
	return result, nil
}
