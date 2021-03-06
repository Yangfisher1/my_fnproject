package server

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/fnproject/fn/api"
	"github.com/fnproject/fn/api/common"
	"github.com/fnproject/fn/api/models"
	"github.com/gin-gonic/gin"
	"go.opencensus.io/tag"
)

// handleHTTPTriggerCall executes the function, for router handlers
func (s *Server) handleHTTPTriggerCall(c *gin.Context) {
	err := s.handleTriggerHTTPFunctionCall2(c)
	if err != nil {
		handleErrorResponse(c, err)
	}
}

func (s *Server) handleHTTPSchedulerCall(c *gin.Context) {
	var stateMachine models.StateMachine
	var inputString string
	body, err := ioutil.ReadAll(c.Request.Body)
	if err != nil {
		handleErrorResponse(c, err)
		return
	}
	err = json.Unmarshal(body, &stateMachine)
	if err != nil {
		handleErrorResponse(c, err)
		return
	}
	if input, ok := c.Request.Header["Input-String"]; ok {
		inputString = input[0]
	} else {
		inputString = ""
	}
	result, err := s.handleStateMachine(c, &stateMachine, &inputString)
	if err != nil {
		handleErrorResponse(c, err)
		return
	}
	c.Writer.WriteString(*result)
}

func (s *Server) benchmark(c *gin.Context) {
	var benchmarkRequest models.BenchmarkRequest
	var inputString string
	body, err := ioutil.ReadAll(c.Request.Body)
	if err != nil {
		handleErrorResponse(c, err)
		return
	}

	err = json.Unmarshal(body, &benchmarkRequest)
	if err != nil {
		handleErrorResponse(c, err)
		return
	}

	fmt.Println("Benchmark requests", benchmarkRequest)

	if input, ok := c.Request.Header["Input-String"]; ok {
		inputString = input[0]
	} else {
		inputString = ""
	}

	var channels []chan models.Checkpoint
	errorChannel := make(chan error, 10)
	start := time.Now().UnixNano()
	for i := uint64(0); i < benchmarkRequest.Count; i++ {
		channel := make(chan models.Checkpoint)
		channels = append(channels, channel)
		go func(finish *chan models.Checkpoint, start int64) {
			defer func() {
				recover()
			}()
			defer func() {
				runtime.UnlockOSThread()
			}()
			runtime.LockOSThread()
			beforeInvoke := time.Now().UnixNano()
			timer := time.After(time.Duration(benchmarkRequest.Time) * time.Second)
			var checkpoints []int64
			var err error
			completedRequest := 0
			errorCount := int64(0)
		SendLoop:
			for {
				_, checkpoints, err = s.syncFunctionInvokeBenchmark(c, getHTTPRequest(inputString), benchmarkRequest.AppName, benchmarkRequest.FuncName)
				if err != nil {
					errorCount++
				} else {
					completedRequest++
				}
				select {
				case <-timer:
					break SendLoop
				default:
				}
			}
			end := time.Now().UnixNano()
			// var elapsedTime []int64
			/*for i := range checkpoints {
				elapsedTime = append(elapsedTime, checkpoints[i]-beforeInvoke)
			}*/
			*finish <- models.Checkpoint{Start: beforeInvoke, End: end, Checkpoints: checkpoints, CompletedRequest: int64(completedRequest), ErrorCount: errorCount}
		}(&channel, start)
	}

	var results []models.Checkpoint
	success := true
	for _, channel := range channels {
		select {
		case result := <-channel:
			results = append(results, result)
		case err := <-errorChannel:
			handleErrorResponse(c, err)
			success = false
			goto end
		}
	}

end:
	for _, channel := range channels {
		close(channel)
	}
	close(errorChannel)

	if success {
		benchmarkResult := models.BenchmarkResult{Checkpoints: results}
		minStart, maxEnd := results[0].Start, results[0].End
		sumLatency := int64(0)
		totalCompletedRequest := int64(0)
		totalError := int64(0)
		for i := range results {
			sumLatency += (results[i].End - results[i].Start)
			if minStart > results[i].Start {
				minStart = results[i].Start
			}
			if maxEnd < results[i].End {
				maxEnd = results[i].End
			}
			totalCompletedRequest += results[i].CompletedRequest
			totalError += results[i].ErrorCount
		}
		benchmarkResult.ElapsedTime = maxEnd - minStart
		// maybe here
		benchmarkResult.AverageLatency = float64(benchmarkResult.ElapsedTime) / float64(uint64(totalCompletedRequest)/benchmarkRequest.Count)
		benchmarkResult.TotalCompletedRequest = totalCompletedRequest
		benchmarkResult.TotalError = totalError
		benchmarkResult.AverageThroughput = float64(totalCompletedRequest) / float64(benchmarkResult.ElapsedTime)

		// which one may be zero?
		fmt.Println("benchmarkResult", benchmarkResult.ElapsedTime, benchmarkResult.AverageLatency, benchmarkResult.TotalCompletedRequest, benchmarkResult.TotalError, benchmarkResult.AverageThroughput)

		s, err := json.Marshal(benchmarkResult)
		if err != nil {
			handleErrorResponse(c, err)
			return
		}
		c.Writer.WriteString(string(s))
	}
}

func mockStateMachine() *models.StateMachine {
	stateMachine := &models.StateMachine{}
	stateMachine.StartAt = "start"
	stateMachine.States = make(map[string]*models.State)
	start := &models.State{}
	end := &models.State{}
	stateMachine.States["start"] = start
	stateMachine.States["end"] = end
	start.Type = "Task"
	start.AppName = "revapp"
	start.FuncName = "/revfunc"
	start.Next = "end"
	start.End = false
	end.Type = "Task"
	end.AppName = "revapp"
	end.FuncName = "/revfunc"
	end.Next = ""
	end.End = true
	return stateMachine
}

func mockInput() string {
	return "Hello"
}

func (s *Server) handleStateMachine(c *gin.Context, stateMachine *models.StateMachine, input *string) (*string, error) {
	current := stateMachine.StartAt
	for {
		if state, ok := stateMachine.States[current]; ok {
			var result *string
			var err error
			var intermediateMap map[string]interface{}
			switch state.Type {
			case models.StateTypeTask:
				result, err = s.handleTask(c, state, input)
				if err != nil {
					return result, err
				}
			case models.StateTypeParallel:
				{
					intermediateMap = make(map[string]interface{})
					err = json.Unmarshal([]byte(*input), &intermediateMap)
					if err != nil {
						return nil, err
					}
					parallelExecution := state.ParallelExecution
					if parallelExecution == nil {
						return nil, fmt.Errorf("a parallel task requires ParallelExecution field")
					}

					iterableItems, ok := intermediateMap[parallelExecution.IterableItemsKey]
					if !ok {
						return nil, fmt.Errorf("required key %s not found", parallelExecution.IterableItemsKey)
					}

					iterableItemsArray, ok := iterableItems.([]interface{})
					if !ok {
						return nil, fmt.Errorf("required []interface{} type, get %T", iterableItems)
					}
					results, err := s.handleParallel(c, &parallelExecution.StateMachine, iterableItemsArray, parallelExecution.IterableItemName)
					if err != nil {
						return nil, err
					}
					jsonBytes, err := json.Marshal(results)
					if err != nil {
						return nil, err
					}
					jsonString := string(jsonBytes)
					result = &jsonString
				}
			default:
				return nil, fmt.Errorf("Unknown state type: %s", state.Type)
			}
			if state.End {
				return result, nil
			}
			input = result
			current = state.Next
		} else {
			return nil, fmt.Errorf("Unknown state name: %s", current)
		}
	}
}

func (s *Server) handleTask(c *gin.Context, state *models.State, input *string) (*string, error) {
	req := getHTTPRequest(*input)
	result, err := s.syncFunctionInvoke(c, req, state.AppName, state.FuncName)
	return result, err
}

func (s *Server) handleParallel(c *gin.Context, stateMachine *models.StateMachine, iterableItems []interface{}, iterableItemName string) ([]*string, error) {
	parallelCount := len(iterableItems)
	resultChannels := make([]chan *string, parallelCount)
	errChannels := make([]chan error, parallelCount)

	for i := range iterableItems {
		resultChannel := make(chan *string)
		errChannel := make(chan error)
		resultChannels[i] = resultChannel
		errChannels[i] = errChannel
		input := make(map[string]interface{})
		input[iterableItemName] = iterableItems[i]
		payload, err := json.Marshal(input)
		if err != nil {
			return nil, err
		}
		inputString := string(payload)
		go func(resultChannel chan *string, errChannel chan error) {
			result, err := s.handleStateMachine(c, stateMachine, &inputString)
			resultChannel <- result
			errChannel <- err
		}(resultChannel, errChannel)
	}

	results := make([]*string, parallelCount)
	errors := make([]error, parallelCount)
	var err error
	for i := 0; i < parallelCount; i++ {
		results[i] = <-resultChannels[i]
		errors[i] = <-errChannels[i]
		if errors[i] != nil {
			err = errors[i]
		}
	}

	return results, err
}

func getHTTPRequest(payload string) *http.Request {
	req := &http.Request{}
	req.Method = "POST"
	req.URL = &url.URL{}
	req.Proto = "HTTP/1.1"
	req.ProtoMajor = 1
	req.ProtoMinor = 1
	body := ioutil.NopCloser(strings.NewReader(payload))
	req.Body = body
	req.GetBody = nil
	req.ContentLength = int64(len(payload))
	req.TransferEncoding = make([]string, 0)
	req.Close = false
	req.Host = ""
	req.Form = make(url.Values)
	req.PostForm = make(url.Values)
	req.MultipartForm = nil
	req.Trailer = make(http.Header)
	req.RemoteAddr = ""
	req.RequestURI = ""
	req.TLS = nil
	req.Cancel = nil
	return req
}

func (s *Server) syncFunctionInvoke(c *gin.Context, req *http.Request, appName string, funcName string) (*string, error) {
	ctx := c.Request.Context()
	appID, err := s.lbReadAccess.GetAppID(ctx, appName)
	if err != nil {
		return nil, err
	}

	app, err := s.lbReadAccess.GetAppByID(ctx, appID)
	if err != nil {
		return nil, err
	}

	trigger, err := s.lbReadAccess.GetTriggerBySource(ctx, appID, "http", funcName)
	if err != nil {
		return nil, err
	}

	fn, err := s.lbReadAccess.GetFnByID(ctx, trigger.FnID)
	if err != nil {
		return nil, err
	}

	requestURL := reqURL(req)
	headers := make(http.Header, 3)
	headers.Set("Fn-Http-Method", req.Method)
	headers.Set("Fn-Http-Request-Url", requestURL)
	headers.Set("Fn-Intent", "httprequest")
	req.Header = headers

	result, err := s.fnInvokeFunctionWithResult(headers, req, app, fn, trigger)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Server) syncFunctionInvokeBenchmark(c *gin.Context, req *http.Request, appName string, funcName string) (*string, []int64, error) {
	var checkpoints []int64
	ctx := c.Request.Context()

	checkpoints = append(checkpoints, time.Now().UnixNano())

	appID, err := s.lbReadAccess.GetAppID(ctx, appName)
	if err != nil {
		return nil, checkpoints, err
	}

	checkpoints = append(checkpoints, time.Now().UnixNano())

	app, err := s.lbReadAccess.GetAppByID(ctx, appID)
	if err != nil {
		return nil, checkpoints, err
	}

	checkpoints = append(checkpoints, time.Now().UnixNano())

	trigger, err := s.lbReadAccess.GetTriggerBySource(ctx, appID, "http", funcName)
	if err != nil {
		return nil, checkpoints, err
	}

	checkpoints = append(checkpoints, time.Now().UnixNano())

	fn, err := s.lbReadAccess.GetFnByID(ctx, trigger.FnID)
	if err != nil {
		return nil, checkpoints, err
	}

	checkpoints = append(checkpoints, time.Now().UnixNano())

	requestURL := reqURL(req)
	headers := make(http.Header, 3)
	headers.Set("Fn-Http-Method", req.Method)
	headers.Set("Fn-Http-Request-Url", requestURL)
	headers.Set("Fn-Intent", "httprequest")
	req.Header = headers

	result, err := s.fnInvokeFunctionWithResult(headers, req, app, fn, trigger)
	if err != nil {
		return nil, checkpoints, err
	}
	return result, checkpoints, nil
}

// handleTriggerHTTPFunctionCall2 executes the function and returns an error
// Requires the following in the context:
func (s *Server) handleTriggerHTTPFunctionCall2(c *gin.Context) error {
	ctx := c.Request.Context()
	p := c.Param(api.TriggerSource)
	if p == "" {
		p = "/"
	}

	appName := c.Param(api.AppName)

	appID, err := s.lbReadAccess.GetAppID(ctx, appName)
	if err != nil {
		return err
	}

	app, err := s.lbReadAccess.GetAppByID(ctx, appID)
	if err != nil {
		return err
	}

	routePath := p

	trigger, err := s.lbReadAccess.GetTriggerBySource(ctx, appID, "http", routePath)

	if err != nil {
		return err
	}

	fn, err := s.lbReadAccess.GetFnByID(ctx, trigger.FnID)
	if err != nil {
		return err
	}
	// gin sets this to 404 on NoRoute, so we'll just ensure it's 200 by default.
	c.Status(200) // this doesn't write the header yet

	err = s.ServeHTTPTrigger(c, app, fn, trigger)
	if models.IsFuncError(err) || err == nil {
		// report all user-directed errors and function responses from here, after submit has run.
		// this is our never ending attempt to distinguish user and platform errors.
		ctx, err := tag.New(c.Request.Context(),
			tag.Upsert(whodunitKey, "user"),
		)
		if err != nil {
			panic(err)
		}
		c.Request = c.Request.WithContext(ctx)
	}
	return err
}

type triggerResponseWriter struct {
	inner     http.ResponseWriter
	committed bool
}

func (trw *triggerResponseWriter) Header() http.Header {
	return trw.inner.Header()
}

func (trw *triggerResponseWriter) Write(b []byte) (int, error) {
	if !trw.committed {
		trw.WriteHeader(http.StatusOK)
	}
	return trw.inner.Write(b)
}

func (trw *triggerResponseWriter) WriteHeader(serviceStatus int) {
	if trw.committed {
		return
	}
	trw.committed = true

	userStatus := 0
	realHeaders := trw.Header()
	gwHeaders := make(http.Header, len(realHeaders))
	for k, vs := range realHeaders {
		switch {
		case strings.HasPrefix(k, "Fn-Http-H-"):
			gwHeader := strings.TrimPrefix(k, "Fn-Http-H-")
			if gwHeader != "" { // case where header is exactly the prefix
				gwHeaders[gwHeader] = vs
			}
		case k == "Fn-Http-Status":
			if len(vs) > 0 {
				statusInt, err := strconv.Atoi(vs[0])
				if err == nil {
					userStatus = statusInt
				}
			}
		case k == "Content-Type", k == "Fn-Call-Id":
			gwHeaders[k] = vs
		}
	}

	// XXX(reed): this is O(3n)... yes sorry for making it work without making it perfect first
	for k := range realHeaders {
		realHeaders.Del(k)
	}
	for k, vs := range gwHeaders {
		realHeaders[k] = vs
	}

	// XXX(reed): simplify / add tests for these behaviors...
	finalStatus := 200
	if serviceStatus >= 400 {
		finalStatus = serviceStatus
	} else if userStatus > 0 {
		finalStatus = userStatus
	}

	trw.inner.WriteHeader(finalStatus)
}

func reqURL(req *http.Request) string {
	if req.URL.Scheme == "" {
		if req.TLS == nil {
			req.URL.Scheme = "http"
		} else {
			req.URL.Scheme = "https"
		}
	}
	if req.URL.Host == "" {
		req.URL.Host = req.Host
	}
	return req.URL.String()
}

// ServeHTTPTrigger serves an HTTP trigger for a given app/fn/trigger based on the current request
// This is exported to allow extensions to handle their own trigger naming and publishing
func (s *Server) ServeHTTPTrigger(c *gin.Context, app *models.App, fn *models.Fn, trigger *models.Trigger) error {
	// transpose trigger headers into the request
	req := c.Request
	headers := make(http.Header, len(req.Header))

	// remove transport headers before decorating headers
	common.StripHopHeaders(req.Header)

	for k, vs := range req.Header {
		switch k {
		case "Content-Type":
		default:
			k = fmt.Sprintf("Fn-Http-H-%s", k)
		}
		headers[k] = vs
	}
	requestURL := reqURL(req)

	headers.Set("Fn-Http-Method", req.Method)
	headers.Set("Fn-Http-Request-Url", requestURL)
	headers.Set("Fn-Intent", "httprequest")
	req.Header = headers

	// trap the headers and rewrite them for http trigger
	rw := &triggerResponseWriter{inner: c.Writer}

	return s.fnInvoke(rw, req, app, fn, trigger)
}
