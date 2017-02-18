package httpsrv

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strconv"

	"github.com/Shopify/sarama"
	"github.com/gorilla/mux"
	"github.com/mailgun/kafka-pixy/actor"
	"github.com/mailgun/kafka-pixy/admin"
	"github.com/mailgun/kafka-pixy/consumer"
	"github.com/mailgun/kafka-pixy/prettyfmt"
	"github.com/mailgun/kafka-pixy/proxy"
	"github.com/mailgun/log"
	"github.com/mailgun/manners"
)

const (
	NetworkTCP  = "tcp"
	NetworkUnix = "unix"

	// HTTP headers used by the API.
	hdrContentLength = "Content-Length"
	hdrContentType   = "Content-Type"

	// HTTP request parameters.
	prmProxy = "proxy"
	prmTopic = "topic"
	prmKey   = "key"
	prmSync  = "sync"
	prmGroup = "group"
)

var (
	EmptyResponse = map[string]interface{}{}
)

type T struct {
	actorID    *actor.ID
	addr       string
	listener   net.Listener
	httpServer *manners.GracefulServer
	proxySet   *proxy.Set
	errorCh    chan error
}

// New creates an HTTP server instance that will accept API requests at the
// specified `network`/`address` and execute them with the specified `producer`,
// `consumer`, or `admin`, depending on the request type.
func New(network, addr string, proxySet *proxy.Set) (*T, error) {
	// Start listening on the specified network/address.
	listener, err := net.Listen(network, addr)
	if err != nil {
		return nil, fmt.Errorf("failed to create listener, err=(%s)", err)
	}
	// If the address is Unix Domain Socket then make it accessible for everyone.
	if network == NetworkUnix {
		if err := os.Chmod(addr, 0777); err != nil {
			return nil, fmt.Errorf("failed to change socket permissions, err=(%s)", err)
		}
	}
	// Create a graceful HTTP server instance.
	router := mux.NewRouter()
	httpServer := manners.NewWithServer(&http.Server{Handler: router})
	hs := &T{
		actorID:    actor.RootID.NewChild(fmt.Sprintf("API@%s", addr)),
		addr:       addr,
		listener:   manners.NewListener(listener),
		httpServer: httpServer,
		proxySet:   proxySet,
		errorCh:    make(chan error, 1),
	}
	// Configure the API request handlers.
	router.HandleFunc(fmt.Sprintf("/proxies/{%s}/topics/{%s}/messages", prmProxy, prmTopic), hs.handleProduce).Methods("POST")
	router.HandleFunc(fmt.Sprintf("/topics/{%s}/messages", prmTopic), hs.handleProduce).Methods("POST")
	router.HandleFunc(fmt.Sprintf("/proxies/{%s}/topics/{%s}/messages", prmProxy, prmTopic), hs.handleProduce).Methods("POST")
	router.HandleFunc(fmt.Sprintf("/topics/{%s}/messages", prmTopic), hs.handleProduce).Methods("POST")
	router.HandleFunc(fmt.Sprintf("/proxies/{%s}/topics/{%s}/messages", prmProxy, prmTopic), hs.handleProduce).Methods("POST")
	router.HandleFunc(fmt.Sprintf("/topics/{%s}/messages", prmTopic), hs.handleConsume).Methods("GET")
	router.HandleFunc(fmt.Sprintf("/proxies/{%s}/topics/{%s}/messages", prmProxy, prmTopic), hs.handleConsume).Methods("GET")
	router.HandleFunc(fmt.Sprintf("/topics/{%s}/offsets", prmTopic), hs.handleGetOffsets).Methods("GET")
	router.HandleFunc(fmt.Sprintf("/proxies/{%s}/topics/{%s}/offsets", prmProxy, prmTopic), hs.handleGetOffsets).Methods("GET")
	router.HandleFunc(fmt.Sprintf("/topics/{%s}/offsets", prmTopic), hs.handleSetOffsets).Methods("POST")
	router.HandleFunc(fmt.Sprintf("/proxies/{%s}/topics/{%s}/offsets", prmProxy, prmTopic), hs.handleSetOffsets).Methods("POST")
	router.HandleFunc(fmt.Sprintf("/topics/{%s}/consumers", prmTopic), hs.handleGetTopicConsumers).Methods("GET")
	router.HandleFunc(fmt.Sprintf("/proxies/{%s}/topics/{%s}/consumers", prmProxy, prmTopic), hs.handleGetTopicConsumers).Methods("GET")
	router.HandleFunc("/_ping", hs.handlePing).Methods("GET")
	return hs, nil
}

// Starts triggers asynchronous HTTP server start. If it fails then the error
// will be sent down to `HTTPAPIServer.ErrorCh()`.
func (s *T) Start() {
	actor.Spawn(s.actorID, nil, func() {
		defer close(s.errorCh)
		if err := s.httpServer.Serve(s.listener); err != nil {
			s.errorCh <- fmt.Errorf("HTTP API listener failed, err=(%s)", err)
		}
	})
}

// ErrorCh returns an output channel that HTTP server running in another
// goroutine will use if it stops with error if one occurs. The channel will be
// closed when the server is fully stopped due to an error or otherwise..
func (s *T) ErrorCh() <-chan error {
	return s.errorCh
}

// AsyncStop triggers HTTP API listener stop. If a caller wants to know when
// the server terminates it should read from the `Error()` channel that will be
// closed upon server termination.
func (s *T) AsyncStop() {
	s.httpServer.Close()
}

func (s *T) getProxy(r *http.Request) *proxy.T {
	pxyAlias := mux.Vars(r)[prmProxy]
	return s.proxySet.Get(pxyAlias)
}

// handleProduce is an HTTP request handler for `POST /topic/{topic}/messages`
func (s *T) handleProduce(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	pxy := s.getProxy(r)
	topic := mux.Vars(r)[prmTopic]
	key := getParamBytes(r, prmKey)
	_, isSync := r.Form[prmSync]

	// Get the message body from the HTTP request.
	if _, ok := r.Header[hdrContentLength]; !ok {
		errorText := fmt.Sprintf("Missing %s header", hdrContentLength)
		respondWithJSON(w, http.StatusBadRequest, errorHTTPResponse{errorText})
		return
	}
	messageSizeStr := r.Header.Get(hdrContentLength)
	messageSize, err := strconv.Atoi(messageSizeStr)
	if err != nil {
		errorText := fmt.Sprintf("Invalid %s header: %s", hdrContentLength, messageSizeStr)
		respondWithJSON(w, http.StatusBadRequest, errorHTTPResponse{errorText})
		return
	}
	message, err := ioutil.ReadAll(r.Body)
	if err != nil {
		errorText := fmt.Sprintf("Failed to read a message: err=(%s)", err)
		respondWithJSON(w, http.StatusBadRequest, errorHTTPResponse{errorText})
		return
	}
	if len(message) != messageSize {
		errorText := fmt.Sprintf("Message size does not match %s: expected=%v, actual=%v",
			hdrContentLength, messageSize, len(message))
		respondWithJSON(w, http.StatusBadRequest, errorHTTPResponse{errorText})
		return
	}

	// Asynchronously submit the message to the Kafka cluster.
	if !isSync {
		pxy.AsyncProduce(topic, toEncoderPreservingNil(key), sarama.StringEncoder(message))
		respondWithJSON(w, http.StatusOK, EmptyResponse)
		return
	}

	prodMsg, err := pxy.Produce(topic, toEncoderPreservingNil(key), sarama.StringEncoder(message))
	if err != nil {
		var status int
		switch err {
		case sarama.ErrUnknownTopicOrPartition:
			status = http.StatusNotFound
		default:
			status = http.StatusInternalServerError
		}
		respondWithJSON(w, status, errorHTTPResponse{err.Error()})
		return
	}

	respondWithJSON(w, http.StatusOK, produceHTTPResponse{
		Partition: prodMsg.Partition,
		Offset:    prodMsg.Offset,
	})
}

// handleConsume is an HTTP request handler for `GET /topic/{topic}/messages`
func (s *T) handleConsume(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	pxy := s.getProxy(r)
	topic := mux.Vars(r)[prmTopic]
	group, err := getGroupParam(r)
	if err != nil {
		respondWithJSON(w, http.StatusBadRequest, errorHTTPResponse{err.Error()})
		return
	}

	consMsg, err := pxy.Consume(group, topic)
	if err != nil {
		var status int
		switch err.(type) {
		case consumer.ErrRequestTimeout:
			status = http.StatusRequestTimeout
		case consumer.ErrTooManyRequests:
			status = http.StatusTooManyRequests
		default:
			status = http.StatusInternalServerError
		}
		respondWithJSON(w, status, errorHTTPResponse{err.Error()})
		return
	}

	respondWithJSON(w, http.StatusOK, consumeHTTPResponse{
		Key:       consMsg.Key,
		Value:     consMsg.Value,
		Partition: consMsg.Partition,
		Offset:    consMsg.Offset,
	})
}

// handleGetOffsets is an HTTP request handler for `GET /topic/{topic}/offsets`
func (s *T) handleGetOffsets(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	pxy := s.getProxy(r)
	topic := mux.Vars(r)[prmTopic]
	group, err := getGroupParam(r)
	if err != nil {
		respondWithJSON(w, http.StatusBadRequest, errorHTTPResponse{err.Error()})
		return
	}

	partitionOffsets, err := pxy.GetGroupOffsets(group, topic)
	if err != nil {
		if err, ok := err.(admin.ErrQuery); ok && err.Cause() == sarama.ErrUnknownTopicOrPartition {
			respondWithJSON(w, http.StatusNotFound, errorHTTPResponse{"Unknown topic"})
			return
		}
		respondWithJSON(w, http.StatusInternalServerError, errorHTTPResponse{err.Error()})
		return
	}

	partitionOffsetView := make([]partitionOffsetView, len(partitionOffsets))
	for i, po := range partitionOffsets {
		partitionOffsetView[i].Partition = po.Partition
		partitionOffsetView[i].Begin = po.Begin
		partitionOffsetView[i].End = po.End
		partitionOffsetView[i].Count = po.End - po.Begin
		partitionOffsetView[i].Offset = po.Offset
		if po.Offset == sarama.OffsetNewest {
			partitionOffsetView[i].Lag = 0
		} else if po.Offset == sarama.OffsetOldest {
			partitionOffsetView[i].Lag = po.End - po.Begin
		} else {
			partitionOffsetView[i].Lag = po.End - po.Offset
		}
		partitionOffsetView[i].Metadata = po.Metadata
	}
	respondWithJSON(w, http.StatusOK, partitionOffsetView)
}

// handleGetOffsets is an HTTP request handler for `POST /topic/{topic}/offsets`
func (s *T) handleSetOffsets(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	pxy := s.getProxy(r)
	topic := mux.Vars(r)[prmTopic]
	group, err := getGroupParam(r)
	if err != nil {
		respondWithJSON(w, http.StatusBadRequest, errorHTTPResponse{err.Error()})
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		errorText := fmt.Sprintf("Failed to read the request: err=(%s)", err)
		respondWithJSON(w, http.StatusBadRequest, errorHTTPResponse{errorText})
		return
	}

	var partitionOffsetViews []partitionOffsetView
	if err := json.Unmarshal(body, &partitionOffsetViews); err != nil {
		errorText := fmt.Sprintf("Failed to parse the request: err=(%s)", err)
		respondWithJSON(w, http.StatusBadRequest, errorHTTPResponse{errorText})
		return
	}

	partitionOffsets := make([]admin.PartitionOffset, len(partitionOffsetViews))
	for i, pov := range partitionOffsetViews {
		partitionOffsets[i].Partition = pov.Partition
		partitionOffsets[i].Offset = pov.Offset
		partitionOffsets[i].Metadata = pov.Metadata
	}

	err = pxy.SetGroupOffsets(group, topic, partitionOffsets)
	if err != nil {
		if err, ok := err.(admin.ErrQuery); ok && err.Cause() == sarama.ErrUnknownTopicOrPartition {
			respondWithJSON(w, http.StatusNotFound, errorHTTPResponse{"Unknown topic"})
			return
		}
		respondWithJSON(w, http.StatusInternalServerError, errorHTTPResponse{err.Error()})
		return
	}

	respondWithJSON(w, http.StatusOK, EmptyResponse)
}

// handleGetTopicConsumers is an HTTP request handler for `GET /topic/{topic}/consumers`
func (s *T) handleGetTopicConsumers(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var err error

	pxy := s.getProxy(r)
	topic := mux.Vars(r)[prmTopic]

	group := ""
	r.ParseForm()
	groups := r.Form[prmGroup]
	if len(groups) > 1 {
		err = fmt.Errorf("One consumer group is expected, but %d provided", len(groups))
		respondWithJSON(w, http.StatusBadRequest, errorHTTPResponse{err.Error()})
		return
	}
	if len(groups) == 1 {
		group = groups[0]
	}

	var consumers map[string]map[string][]int32
	if group == "" {
		consumers, err = pxy.GetAllTopicConsumers(topic)
		if err != nil {
			respondWithJSON(w, http.StatusInternalServerError, errorHTTPResponse{err.Error()})
			return
		}
	} else {
		groupConsumers, err := pxy.GetTopicConsumers(group, topic)
		if err != nil {
			if _, ok := err.(admin.ErrInvalidParam); ok {
				respondWithJSON(w, http.StatusBadRequest, errorHTTPResponse{err.Error()})
				return
			}
			respondWithJSON(w, http.StatusInternalServerError, errorHTTPResponse{err.Error()})
			return
		}
		consumers = make(map[string]map[string][]int32)
		if len(groupConsumers) != 0 {
			consumers[group] = groupConsumers
		}
	}

	encodedRes, err := json.MarshalIndent(consumers, "", "  ")
	if err != nil {
		log.Errorf("Failed to send HTTP response: status=%d, body=%v, reason=%v", http.StatusOK, encodedRes, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	encodedRes = prettyfmt.CollapseJSON(encodedRes)

	w.Header().Add(hdrContentType, "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(encodedRes); err != nil {
		log.Errorf("Failed to send HTTP response: status=%d, body=%v, reason=%v", http.StatusOK, encodedRes, err)
	}
}

func (s *T) handlePing(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("pong"))
}

type produceHTTPResponse struct {
	Partition int32 `json:"partition"`
	Offset    int64 `json:"offset"`
}

type consumeHTTPResponse struct {
	Key       []byte `json:"key"`
	Value     []byte `json:"value"`
	Partition int32  `json:"partition"`
	Offset    int64  `json:"offset"`
}

type partitionOffsetView struct {
	Partition int32  `json:"partition"`
	Begin     int64  `json:"begin"`
	End       int64  `json:"end"`
	Count     int64  `json:"count"`
	Offset    int64  `json:"offset"`
	Lag       int64  `json:"lag"`
	Metadata  string `json:"metadata,omitempty"`
}

type errorHTTPResponse struct {
	Error string `json:"error"`
}

// getParamBytes returns the request parameter s a slice of bytes. It works
// pretty much the same way s `http.FormValue`, except it distinguishes empty
// value (`[]byte{}`) from missing one (`nil`).
func getParamBytes(r *http.Request, name string) []byte {
	r.ParseForm() // Ignore errors, the go library does the same in FormValue.
	values, ok := r.Form[name]
	if !ok || len(values) == 0 {
		return nil
	}
	return []byte(values[0])
}

// respondWithJSON marshals `body` to a JSON string and sends it s an HTTP
// response body along with the specified `status` code.
func respondWithJSON(w http.ResponseWriter, status int, body interface{}) {
	encodedRes, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		log.Errorf("Failed to send HTTP response: status=%d, body=%v, reason=%v", status, body, err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Add(hdrContentType, "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(encodedRes); err != nil {
		log.Errorf("Failed to send HTTP response: status=%d, body=%v, reason=%v", status, body, err)
	}
}

func getGroupParam(r *http.Request) (string, error) {
	r.ParseForm()
	groups := r.Form[prmGroup]
	if len(groups) != 1 {
		return "", fmt.Errorf("One consumer group is expected, but %d provided", len(groups))
	}
	return groups[0], nil
}

// toEncoderPreservingNil converts a slice of bytes to `sarama.Encoder` but
// returns `nil` if the passed slice is `nil`.
func toEncoderPreservingNil(b []byte) sarama.Encoder {
	if b != nil {
		return sarama.StringEncoder(b)
	}
	return nil
}
