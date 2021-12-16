package server

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/fnproject/fn/api"
	"github.com/fnproject/fn/api/agent"
	"github.com/fnproject/fn/api/common"
	"github.com/fnproject/fn/api/models"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/tag"
)

var (
	bufPool = &sync.Pool{New: func() interface{} { return new(bytes.Buffer) }}
)

// ResponseBuffer  implements http.ResponseWriter
type ResponseBuffer interface {
	http.ResponseWriter
	Status() int
}

// implements http.ResponseWriter
// this little guy buffers responses from user containers and lets them still
// set headers and such without us risking writing partial output [as much, the
// server could still die while we're copying the buffer]. this lets us set
// content length and content type nicely, as a bonus. it is sad, yes.
type syncResponseWriter struct {
	headers http.Header
	status  int
	*bytes.Buffer
}

var _ http.ResponseWriter = new(syncResponseWriter) // nice compiler errors

func (s *syncResponseWriter) Header() http.Header  { return s.headers }
func (s *syncResponseWriter) WriteHeader(code int) { s.status = code }
func (s *syncResponseWriter) Status() int          { return s.status }

func ReadCSV(filepath string) []string {
	file, err := os.Open(filepath)
	if err != nil {
		fmt.Println(err)
		return nil
	}
	defer file.Close()
	w := csv.NewReader(file)
	data, err := w.Read()
	if err != nil {
		fmt.Println(err)
		return nil
	}
	return data
}

// handleFnInvokeCall executes the function, for router handlers
func (s *Server) handleFnInvokeCall(c *gin.Context) {
	fnID := c.Param(api.FnID)
	ctx, _ := common.LoggerWithFields(c.Request.Context(), logrus.Fields{"fn_id": fnID})
	c.Request = c.Request.WithContext(ctx)
	err := s.handleFnInvokeCall2(c)
	if err != nil {
		handleErrorResponse(c, err)
	}
}

func (s *Server) handleCache(c *gin.Context) {
	// fnID := c.Param(api.FnID)
	// SpikeCnt := c.Param(api.SpikeCnt)
	// in this case, SpikeCnt should be "/156138", so we need to transform it

	// string_cnt := SpikeCnt[1:]
	// cnt, _ := strconv.Atoi(string_cnt)
	// var invoke_data []string
	// use the first csv config
	// if cnt == 1 {
	// 	invoke_data = ReadCSV("/home/yyh/request_burst/1.csv")
	// } else if cnt == 2 {
	// 	invoke_data = ReadCSV("/home/yyh/request_burst/2.csv")
	// } else if cnt == 3 {
	// 	// this is used for temp test
	// 	invoke_data = ReadCSV("/home/yyh/request_burst/3.csv")
	// }
	invoke_data := [...]string{"240", "240", "240", "240", "240", "240", "240", "240", "240", "240"}

	// after we have some hot container's, we can invoke them at some interval
	invoke_data2 := [...]string{"480", "480", "480", "480", "480", "480", "480", "480", "480", "480"}
	// generate request data by ourselves rather than read a csv file

	// every minute
	request_minute := 720
	first_cnt := 240

	thpt_cnt := make([]int, request_minute)
	prev_thpt := 0
	test_minute := 10
	thpt_result := make([]int, 60*test_minute)

	latency := make([]float64, test_minute)
	// fmt.Println(invoke_data)
	// simulate, all together there are 1440 minutes, here 1 minute = 0.25 second, so 240x

	// prepare for it
	all_latency := make([][]float64, test_minute)

	// Should we load the spike as a concurrent way? I don't think it's reasonable
	var wg sync.WaitGroup
	wg.Add(test_minute)

	// just send out all the information we need at once, so we have more opportunity to get hot container or launch new container
	// first try sendout at once, later try another way
	go func() {
		for second := 0; second < test_minute; second++ {
			now_second := second
			all_latency[now_second] = make([]float64, request_minute)
			go func() {
				request_cnt, _ := strconv.Atoi(invoke_data[now_second])
				// second_request := request_cnt / 60
				// real_req_cnt := second_request * 60

				var wg2 sync.WaitGroup
				// !!! we don't have exactly request_cnt go routine, due to the divide we lost some
				// we have `request_cnt` invoke, so we have `request_cnt` latency
				// simulate every second, as an average

				// if there is no request in this minute, we won't need to create go routine and sleep for a while

				// send out at once
				if request_cnt != 0 {
					wg2.Add(request_cnt)
					for i := 0; i < request_cnt; i++ {
						request_seq := i
						go func() {
							defer wg2.Done()
							start_time := time.Now().UnixNano()
							ctx, _ := common.LoggerWithFields(c.Request.Context(), logrus.Fields{})
							c.Request = c.Request.WithContext(ctx)
							err := s.handleSpikeCall2(c)
							end_time := time.Now().UnixNano()
							thpt_cnt[request_cnt] += 1
							req_latency := float64(end_time) - float64(start_time)
							all_latency[now_second][request_seq] = req_latency / 1000000
							if err != nil {
								handleErrorResponse(c, err)
							}
						}()
					}
				} else if request_cnt == 0 {
					// do nothing
				}
				wg2.Wait()

				// if request_cnt == 0 {
				// 	latency[now_second] = 0
				// } else {
				// 	sum := 0.0
				// 	for _, v := range all_latency[now_second] {
				// 		sum += v
				// 	}
				// 	latency[now_second] = sum / float64(request_cnt)
				// }

				fmt.Println(now_second, "is half done.")
			}()

			time.Sleep(15 * time.Second)

			go func() {
				defer wg.Done()
				request_cnt, _ := strconv.Atoi(invoke_data2[now_second])
				// second_request := request_cnt / 60
				// real_req_cnt := second_request * 60

				var wg2 sync.WaitGroup
				// !!! we don't have exactly request_cnt go routine, due to the divide we lost some
				// we have `request_cnt` invoke, so we have `request_cnt` latency
				// simulate every second, as an average

				// if there is no request in this minute, we won't need to create go routine and sleep for a while

				// send out at once
				if request_cnt != 0 {
					wg2.Add(request_cnt)
					for i := 0; i < request_cnt; i++ {
						request_seq := i
						go func() {
							defer wg2.Done()
							start_time := time.Now().UnixNano()
							ctx, _ := common.LoggerWithFields(c.Request.Context(), logrus.Fields{})
							c.Request = c.Request.WithContext(ctx)
							err := s.handleSpikeCall2(c)
							end_time := time.Now().UnixNano()
							thpt_cnt[first_cnt+request_seq] += 1
							req_latency := float64(end_time) - float64(start_time)
							all_latency[now_second][first_cnt+request_seq] = req_latency / 1000000
							if err != nil {
								handleErrorResponse(c, err)
							}
						}()
					}
				} else if request_cnt == 0 {
					// do nothing
				}
				wg2.Wait()

				if request_cnt == 0 {
					latency[now_second] = 0
				} else {
					sum := 0.0
					for _, v := range all_latency[now_second] {
						sum += v
					}
					latency[now_second] = sum / float64(request_cnt)
				}
				fmt.Println(now_second, "is done.")
			}()

			time.Sleep(45 * time.Second)
		}
	}()

	for second := 0; second < 60*test_minute; second++ {
		now_cnt := 0
		for _, value := range thpt_cnt {
			now_cnt += value
		}
		thpt_result[second] = now_cnt - prev_thpt
		prev_thpt = now_cnt

		time.Sleep(time.Second)
	}

	wg.Wait()
	fmt.Println("All spike loaded.")

	// now write the latency data into a file
	file, err := os.Create("/home/yyh/request_burst/output.txt")
	if err != nil {
		fmt.Println("Write Answer Error", err)
		handleErrorResponse(c, err)
	}

	defer file.Close()

	fileWriter := bufio.NewWriter(file)
	for _, v := range latency {
		fileWriter.WriteString(fmt.Sprint(v))
		fileWriter.WriteString("\n")
		fileWriter.Flush()
	}

	// and write all the latency's into another file to draw a cdf graph
	file2, err := os.Create("/home/yyh/request_burst/latency.txt")
	if err != nil {
		fmt.Println("Write Latency Error", err)
		handleErrorResponse(c, err)
	}

	defer file2.Close()

	fileWriter2 := bufio.NewWriter(file2)
	for _, v := range all_latency {
		for _, l := range v {
			fileWriter2.WriteString(fmt.Sprint(l))
			fileWriter2.WriteString("\n")
			fileWriter2.Flush()
		}
	}

	// and write all the latency's into another file to draw a cdf graph
	file3, err := os.Create("/home/yyh/request_burst/thpt.txt")
	if err != nil {
		fmt.Println("Write thpt Error", err)
		handleErrorResponse(c, err)
	}

	defer file3.Close()

	fileWriter3 := bufio.NewWriter(file3)
	for _, v := range thpt_result {
		fileWriter3.WriteString(fmt.Sprint(v))
		fileWriter3.WriteString("\n")
		fileWriter3.Flush()
	}

	fmt.Println("All Task Done")
}

func (s *Server) handleRealSpike(c *gin.Context) {
	fnID := c.Param(api.FnID)
	SpikeCnt := c.Param(api.SpikeCnt)
	// in this case, SpikeCnt should be "/156138", so we need to transform it

	string_cnt := SpikeCnt[1:]
	cnt, _ := strconv.Atoi(string_cnt)
	var invoke_data []string
	// use the first csv config
	if cnt == 1 {
		invoke_data = ReadCSV("/home/yyh/request_burst/1.csv")
	} else if cnt == 2 {
		invoke_data = ReadCSV("/home/yyh/request_burst/2.csv")
	} else if cnt == 3 {
		// this is used for temp test
		invoke_data = ReadCSV("/home/yyh/request_burst/3.csv")
	}

	test_minute := 10

	latency := make([]float64, test_minute)
	// fmt.Println(invoke_data)
	// simulate, all together there are 1440 minutes, here 1 minute = 0.25 second, so 240x

	// prepare for it
	all_latency := make([][]float64, test_minute)

	// Should we load the spike as a concurrent way? I don't think it's reasonable
	var wg sync.WaitGroup
	wg.Add(test_minute)

	fmt.Println("Test1")
	for second := 0; second < test_minute; second++ {
		now_second := second
		go func() {
			defer wg.Done()
			request_cnt, _ := strconv.Atoi(invoke_data[now_second])
			request_cnt /= 4
			second_request := request_cnt / 60
			real_req_cnt := second_request * 60

			var wg2 sync.WaitGroup
			// !!! we don't have exactly request_cnt go routine, due to the divide we lost some
			// we have `request_cnt` invoke, so we have `request_cnt` latency
			// simulate every second, as an average

			// if there is no request in this minute, we won't need to create go routine and sleep for a while

			// send out at once
			if request_cnt < 60 {
				all_latency[now_second] = make([]float64, request_cnt)
				wg2.Add(request_cnt)
				for i := 0; i < request_cnt; i++ {
					request_seq := i
					go func() {
						defer wg2.Done()
						start_time := time.Now().UnixNano()
						ctx, _ := common.LoggerWithFields(c.Request.Context(), logrus.Fields{})
						c.Request = c.Request.WithContext(ctx)
						err := s.handleSpikeCall2(c)
						end_time := time.Now().UnixNano()
						req_latency := float64(end_time) - float64(start_time)
						all_latency[now_second][request_seq] = req_latency / 1000000
						if err != nil {
							handleErrorResponse(c, err)
						}
					}()
				}
			} else if request_cnt == 0 {
				// do nothing

			} else if request_cnt > 60 {
				all_latency[now_second] = make([]float64, real_req_cnt)
				wg2.Add(real_req_cnt)
				for j := 0; j < 60; j++ {
					real_second := j
					go func() {
						for i := 0; i < second_request; i++ {
							request_seq := i
							go func() {
								defer wg2.Done()
								start_time := time.Now().UnixNano()
								ctx, _ := common.LoggerWithFields(c.Request.Context(), logrus.Fields{"fn_id": fnID})
								c.Request = c.Request.WithContext(ctx)
								err := s.handleSpikeCall2(c)
								end_time := time.Now().UnixNano()
								req_latency := float64(end_time) - float64(start_time)
								all_latency[now_second][real_second*second_request+request_seq] = req_latency / 1000000
								if err != nil {
									handleErrorResponse(c, err)
								}
							}()
						}
					}()
					time.Sleep(time.Second)
				}
			}
			wg2.Wait()

			// Normal way, Serail Send
			// for i := 0; i < request_cnt; i++ {
			// 	ctx, _ := common.LoggerWithFields(c.Request.Context(), logrus.Fields{"fn_id": fnID})
			// 	c.Request = c.Request.WithContext(ctx)
			// 	err := s.handleSpikeCall2(c)
			// 	if err != nil {
			// 		handleErrorResponse(c, err)
			// 	}
			// }

			// calculate avg_laetncy
			if request_cnt == 0 {
				latency[now_second] = 0
			} else {
				sum := 0.0
				for _, v := range all_latency[now_second] {
					sum += v
				}
				latency[now_second] = sum / float64(request_cnt)
			}

			fmt.Println(now_second, "is done.")
		}()
		time.Sleep(60 * time.Second)
	}

	wg.Wait()
	fmt.Println("All spike loaded.")

	// now write the latency data into a file
	file, err := os.Create("/home/yyh/request_burst/output.txt")
	if err != nil {
		fmt.Println("Write Answer Error", err)
		handleErrorResponse(c, err)
	}

	defer file.Close()

	fileWriter := bufio.NewWriter(file)
	for _, v := range latency {
		fileWriter.WriteString(fmt.Sprint(v))
		fileWriter.WriteString("\n")
		fileWriter.Flush()
	}

	// and write all the latency's into another file to draw a cdf graph
	file2, err := os.Create("/home/yyh/request_burst/latency.txt")
	if err != nil {
		fmt.Println("Write Latency Error", err)
		handleErrorResponse(c, err)
	}

	defer file2.Close()

	fileWriter2 := bufio.NewWriter(file2)
	for _, v := range all_latency {
		for _, l := range v {
			fileWriter2.WriteString(fmt.Sprint(l))
			fileWriter2.WriteString("\n")
			fileWriter2.Flush()
		}
	}

	fmt.Println("All Task Done")
	// // start time
	// start_time := time.Now().UnixNano()
	// // invoke, invoke and invoke
	// for i := 0; i < cnt; i++ {
	// 	ctx, _ := common.LoggerWithFields(c.Request.Context(), logrus.Fields{"fn_id": fnID})
	// 	c.Request = c.Request.WithContext(ctx)
	// 	err := s.handleSpikeCall2(c)
	// 	if err != nil {
	// 		handleErrorResponse(c, err)
	// 	}
	// }
	// end_time := time.Now().UnixNano()
	// avg_latency := (float64(end_time) - float64(start_time)) / 1000000
	// fmt.Println("avg latency is", avg_latency)
}

func (s *Server) handleRealSpike2(c *gin.Context) {
	// fnID := c.Param(api.FnID)
	SpikeCnt := c.Param(api.SpikeCnt)
	// in this case, SpikeCnt should be "/156138", so we need to transform it

	string_cnt := SpikeCnt[1:]
	cnt, _ := strconv.Atoi(string_cnt)
	var invoke_data []string
	// use the first csv config
	if cnt == 1 {
		invoke_data = ReadCSV("/home/yyh/request_burst/1.csv")
	} else if cnt == 2 {
		invoke_data = ReadCSV("/home/yyh/request_burst/2.csv")
	} else if cnt == 3 {
		// this is used for temp test
		invoke_data = ReadCSV("/home/yyh/request_burst/3.csv")
	}

	test_minute := 10

	latency := make([]float64, test_minute)
	// fmt.Println(invoke_data)
	// simulate, all together there are 1440 minutes, here 1 minute = 0.25 second, so 240x

	// prepare for it
	all_latency := make([][]float64, test_minute)

	// Should we load the spike as a concurrent way? I don't think it's reasonable
	var wg sync.WaitGroup
	wg.Add(test_minute)

	// just send out all the information we need at once, so we have more opportunity to get hot container or launch new container
	// first try sendout at once, later try another way
	for second := 0; second < test_minute; second++ {
		now_second := second
		go func() {
			defer wg.Done()
			request_cnt, _ := strconv.Atoi(invoke_data[now_second])
			request_cnt /= 4
			// second_request := request_cnt / 60
			// real_req_cnt := second_request * 60

			var wg2 sync.WaitGroup
			// !!! we don't have exactly request_cnt go routine, due to the divide we lost some
			// we have `request_cnt` invoke, so we have `request_cnt` latency
			// simulate every second, as an average

			// if there is no request in this minute, we won't need to create go routine and sleep for a while

			// send out at once
			if request_cnt != 0 {
				all_latency[now_second] = make([]float64, request_cnt)
				wg2.Add(request_cnt)
				for i := 0; i < request_cnt; i++ {
					request_seq := i
					go func() {
						defer wg2.Done()
						start_time := time.Now().UnixNano()
						ctx, _ := common.LoggerWithFields(c.Request.Context(), logrus.Fields{})
						c.Request = c.Request.WithContext(ctx)
						err := s.handleSpikeCall2(c)
						end_time := time.Now().UnixNano()
						req_latency := float64(end_time) - float64(start_time)
						all_latency[now_second][request_seq] = req_latency / 1000000
						if err != nil {
							handleErrorResponse(c, err)
						}
					}()
				}
			} else if request_cnt == 0 {
				// do nothing
			}
			wg2.Wait()

			// Normal way, Serail Send
			// for i := 0; i < request_cnt; i++ {
			// 	ctx, _ := common.LoggerWithFields(c.Request.Context(), logrus.Fields{"fn_id": fnID})
			// 	c.Request = c.Request.WithContext(ctx)
			// 	err := s.handleSpikeCall2(c)
			// 	if err != nil {
			// 		handleErrorResponse(c, err)
			// 	}
			// }

			// calculate avg_laetncy
			if request_cnt == 0 {
				latency[now_second] = 0
			} else {
				sum := 0.0
				for _, v := range all_latency[now_second] {
					sum += v
				}
				latency[now_second] = sum / float64(request_cnt)
			}

			fmt.Println(now_second, "is done.")
		}()
		time.Sleep(60 * time.Second)
	}

	wg.Wait()
	fmt.Println("All spike loaded.")

	// now write the latency data into a file
	file, err := os.Create("/home/yyh/request_burst/output.txt")
	if err != nil {
		fmt.Println("Write Answer Error", err)
		handleErrorResponse(c, err)
	}

	defer file.Close()

	fileWriter := bufio.NewWriter(file)
	for _, v := range latency {
		fileWriter.WriteString(fmt.Sprint(v))
		fileWriter.WriteString("\n")
		fileWriter.Flush()
	}

	// and write all the latency's into another file to draw a cdf graph
	file2, err := os.Create("/home/yyh/request_burst/latency.txt")
	if err != nil {
		fmt.Println("Write Latency Error", err)
		handleErrorResponse(c, err)
	}

	defer file2.Close()

	fileWriter2 := bufio.NewWriter(file2)
	for _, v := range all_latency {
		for _, l := range v {
			fileWriter2.WriteString(fmt.Sprint(l))
			fileWriter2.WriteString("\n")
			fileWriter2.Flush()
		}
	}

	fmt.Println("All Task Done")
	// // start time
	// start_time := time.Now().UnixNano()
	// // invoke, invoke and invoke
	// for i := 0; i < cnt; i++ {
	// 	ctx, _ := common.LoggerWithFields(c.Request.Context(), logrus.Fields{"fn_id": fnID})
	// 	c.Request = c.Request.WithContext(ctx)
	// 	err := s.handleSpikeCall2(c)
	// 	if err != nil {
	// 		handleErrorResponse(c, err)
	// 	}
	// }
	// end_time := time.Now().UnixNano()
	// avg_latency := (float64(end_time) - float64(start_time)) / 1000000
	// fmt.Println("avg latency is", avg_latency)
}

func (s *Server) handleSpikeCall(c *gin.Context) {
	fnID := c.Param(api.FnID)
	SpikeCnt := c.Param(api.SpikeCnt)
	// in this case, SpikeCnt should be "/156138", so we need to transform it

	string_cnt := SpikeCnt[1:]
	cnt, _ := strconv.Atoi(string_cnt)
	var invoke_data []string
	// use the first csv config
	if cnt == 1 {
		invoke_data = ReadCSV("/home/yyh/request_burst/1.csv")
	} else if cnt == 2 {
		invoke_data = ReadCSV("/home/yyh/request_burst/2.csv")
	} else if cnt == 3 {
		// this is used for temp test
		invoke_data = ReadCSV("/home/yyh/request_burst/3.csv")
	}

	latency := make([]float64, 1440)
	// fmt.Println(invoke_data)
	// simulate, all together there are 1440 minutes, here 1 minute = 0.25 second, so 240x

	// prepare for it
	all_latency := make([][]float64, 1440)

	// Should we load the spike as a concurrent way? I don't think it's reasonable
	var wg sync.WaitGroup
	wg.Add(1440)
	for second := 0; second < 1440; second++ {
		now_second := second
		go func() {
			defer wg.Done()

			request_cnt, _ := strconv.Atoi(invoke_data[now_second])

			// need more tricky
			if request_cnt < 240 && request_cnt != 0 {
				request_cnt = 1
			} else {
				request_cnt /= 240
			}

			all_latency[now_second] = make([]float64, request_cnt)

			var wg2 sync.WaitGroup
			wg2.Add(request_cnt)
			// we have `request_cnt` invoke, so we have `request_cnt` latency
			for i := 0; i < request_cnt; i++ {
				request_seq := i
				go func() {
					defer wg2.Done()
					start_time := time.Now().UnixNano()
					ctx, _ := common.LoggerWithFields(c.Request.Context(), logrus.Fields{"fn_id": fnID})
					c.Request = c.Request.WithContext(ctx)
					err := s.handleSpikeCall2(c)
					end_time := time.Now().UnixNano()
					req_latency := float64(end_time) - float64(start_time)
					all_latency[now_second][request_seq] = req_latency / 1000000
					if err != nil {
						handleErrorResponse(c, err)
					}
				}()
			}
			wg2.Wait()

			// Normal way, Serail Send
			// for i := 0; i < request_cnt; i++ {
			// 	ctx, _ := common.LoggerWithFields(c.Request.Context(), logrus.Fields{"fn_id": fnID})
			// 	c.Request = c.Request.WithContext(ctx)
			// 	err := s.handleSpikeCall2(c)
			// 	if err != nil {
			// 		handleErrorResponse(c, err)
			// 	}
			// }

			// calculate avg_laetncy
			if request_cnt == 0 {
				latency[now_second] = 0
			} else {
				sum := 0.0
				for _, v := range all_latency[now_second] {
					sum += v
				}
				latency[now_second] = sum / float64(request_cnt)
			}

			fmt.Println(now_second, "is done.")
		}()
		time.Sleep(250 * time.Millisecond)
	}

	wg.Wait()
	fmt.Println("All spike loaded.")

	// now write the latency data into a file
	file, err := os.Create("/home/yyh/request_burst/output.txt")
	if err != nil {
		fmt.Println("Write Answer Error", err)
		handleErrorResponse(c, err)
	}

	defer file.Close()

	fileWriter := bufio.NewWriter(file)
	for _, v := range latency {
		fileWriter.WriteString(fmt.Sprint(v))
		fileWriter.WriteString("\n")
		fileWriter.Flush()
	}

	// and write all the latency's into another file to draw a cdf graph
	file2, err := os.Create("/home/yyh/request_burst/latency.txt")
	if err != nil {
		fmt.Println("Write Latency Error", err)
		handleErrorResponse(c, err)
	}

	defer file2.Close()

	fileWriter2 := bufio.NewWriter(file2)
	for _, v := range all_latency {
		for _, l := range v {
			fileWriter2.WriteString(fmt.Sprint(l))
			fileWriter2.WriteString("\n")
			fileWriter2.Flush()
		}
	}

	fmt.Println("All Task Done")
	// // start time
	// start_time := time.Now().UnixNano()
	// // invoke, invoke and invoke
	// for i := 0; i < cnt; i++ {
	// 	ctx, _ := common.LoggerWithFields(c.Request.Context(), logrus.Fields{"fn_id": fnID})
	// 	c.Request = c.Request.WithContext(ctx)
	// 	err := s.handleSpikeCall2(c)
	// 	if err != nil {
	// 		handleErrorResponse(c, err)
	// 	}
	// }
	// end_time := time.Now().UnixNano()
	// avg_latency := (float64(end_time) - float64(start_time)) / 1000000
	// fmt.Println("avg latency is", avg_latency)

}

func (s *Server) handleSpikeCall2(c *gin.Context) error {
	ctx := c.Request.Context()
	fn, err := s.lbReadAccess.GetFnByID(ctx, c.Param(api.FnID))
	if err != nil {
		return err
	}

	app, err := s.lbReadAccess.GetAppByID(ctx, fn.AppID)
	if err != nil {
		return err
	}

	err = s.ServeFnSpike(c, app, fn)
	// if models.IsFuncError(err) || err == nil {
	// 	// report all user-directed errors and function responses from here, after submit has run.
	// 	// this is our never ending attempt to distinguish user and platform errors.
	// 	ctx, err := tag.New(ctx,
	// 		tag.Insert(whodunitKey, "user"),
	// 	)
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// 	c.Request = c.Request.WithContext(ctx)
	// }
	return err
}

func (s *Server) ServeFnSpike(c *gin.Context, app *models.App, fn *models.Fn) error {
	return s.fnSpike(c.Writer, c.Request, app, fn, nil)
}

func (s *Server) fnSpike(resp http.ResponseWriter, req *http.Request, app *models.App, fn *models.Fn, trig *models.Trigger) error {
	// TODO: we should get rid of the buffers, and stream back (saves memory (+splice), faster (splice), allows streaming, don't have to cap resp size)
	// buffer the response before writing it out to client to prevent partials from trying to stream
	// fmt.Printf("http.Request: %v\n", *req)
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	var writer ResponseBuffer

	// isDetached := req.Header.Get("Fn-Invoke-Type") == models.TypeDetached
	// if isDetached {
	// 	writer = agent.NewDetachedResponseWriter(resp.Header(), 202)
	// } else {
	// 	writer = &syncResponseWriter{
	// 		headers: resp.Header(),
	// 		status:  200,
	// 		Buffer:  buf,
	// 	}
	// }
	writer = &syncResponseWriter{
		headers: resp.Header(),
		status:  200,
		Buffer:  buf,
	}

	opts := getCallOptions(req, app, fn, trig, writer)

	call, err := s.agent.GetCall(opts...)
	if err != nil {
		return err
	}

	// add this before submit, always tie a call id to the response at this point
	// writer.Header().Add("Fn-Call-Id", call.Model().ID)

	err = s.agent.Submit(call)
	if err != nil {
		return err
	}

	// because we can...
	// writer.Header().Set("Content-Length", strconv.Itoa(int(buf.Len())))

	// buffered response writer traps status (so we can add headers), we need to write it still
	// if writer.Status() > 0 {
	// 	resp.WriteHeader(writer.Status())
	// }

	// if isDetached {
	// 	return nil
	// }

	// fmt.Print("Function returns: ")
	// fmt.Println(buf.String())
	// io.Copy(resp, buf)
	// bufPool.Put(buf) // at this point, submit returned without timing out, so we can re-use this one
	return nil
}

// handleTriggerHTTPFunctionCall2 executes the function and returns an error
// Requires the following in the context:
func (s *Server) handleFnInvokeCall2(c *gin.Context) error {
	ctx := c.Request.Context()
	fn, err := s.lbReadAccess.GetFnByID(ctx, c.Param(api.FnID))
	if err != nil {
		return err
	}

	app, err := s.lbReadAccess.GetAppByID(ctx, fn.AppID)
	if err != nil {
		return err
	}

	err = s.ServeFnInvoke(c, app, fn)
	if models.IsFuncError(err) || err == nil {
		// report all user-directed errors and function responses from here, after submit has run.
		// this is our never ending attempt to distinguish user and platform errors.
		ctx, err := tag.New(ctx,
			tag.Insert(whodunitKey, "user"),
		)
		if err != nil {
			panic(err)
		}
		c.Request = c.Request.WithContext(ctx)
	}
	return err
}

func (s *Server) ServeFnInvoke(c *gin.Context, app *models.App, fn *models.Fn) error {
	return s.fnInvoke(c.Writer, c.Request, app, fn, nil)
}

func (s *Server) fnInvoke(resp http.ResponseWriter, req *http.Request, app *models.App, fn *models.Fn, trig *models.Trigger) error {
	// TODO: we should get rid of the buffers, and stream back (saves memory (+splice), faster (splice), allows streaming, don't have to cap resp size)
	// buffer the response before writing it out to client to prevent partials from trying to stream
	fmt.Printf("http.Request: %v\n", *req)
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	var writer ResponseBuffer

	isDetached := req.Header.Get("Fn-Invoke-Type") == models.TypeDetached
	if isDetached {
		writer = agent.NewDetachedResponseWriter(resp.Header(), 202)
	} else {
		writer = &syncResponseWriter{
			headers: resp.Header(),
			status:  200,
			Buffer:  buf,
		}
	}
	opts := getCallOptions(req, app, fn, trig, writer)

	call, err := s.agent.GetCall(opts...)
	if err != nil {
		return err
	}

	// add this before submit, always tie a call id to the response at this point
	writer.Header().Add("Fn-Call-Id", call.Model().ID)

	err = s.agent.Submit(call)
	if err != nil {
		return err
	}

	// because we can...
	writer.Header().Set("Content-Length", strconv.Itoa(int(buf.Len())))

	// buffered response writer traps status (so we can add headers), we need to write it still
	if writer.Status() > 0 {
		resp.WriteHeader(writer.Status())
	}

	if isDetached {
		return nil
	}

	fmt.Print("Function returns: ")
	fmt.Println(buf.String())
	io.Copy(resp, buf)
	bufPool.Put(buf) // at this point, submit returned without timing out, so we can re-use this one
	return nil
}

func (s *Server) randomInvoke(c *gin.Context) {
	// maybe we can read a CDF graph or invoke csv to do random invoke
	SpikeCnt := c.Param(api.SpikeCnt)
	// in this case, SpikeCnt should be "/156138", so we need to transform it

	fnIDs := [...]string{"01FPM8RJRWNG8G00GZJ0000001", "01F5YHB449NG8G00GZJ0000003", "01F5AMYMD8NG8G00GZJ0000005", "01EQT25TYZNG8G00GZJ0000002", "01F5X1T8WJNG8G00GZJ0000001", "01FPM9R94KNG8G00GZJ0000002"}
	string_cnt := SpikeCnt[1:]
	cnt, _ := strconv.Atoi(string_cnt)
	var invoke_data []string
	// use the first csv config
	if cnt == 1 {
		invoke_data = ReadCSV("/home/yyh/request_burst/1.csv")
	} else if cnt == 2 {
		invoke_data = ReadCSV("/home/yyh/request_burst/2.csv")
	} else if cnt == 3 {
		// this is used for temp test
		invoke_data = ReadCSV("/home/yyh/request_burst/3.csv")
	}

	// Should we load the spike as a concurrent way? I don't think it's reasonable
	var wg sync.WaitGroup
	wg.Add(1440)
	for second := 0; second < 1440; second++ {
		now_second := second
		go func() {
			defer wg.Done()

			request_cnt, _ := strconv.Atoi(invoke_data[now_second])

			// need more tricky
			if request_cnt != 0 {
				// generate random noise, range 10-100 req/0.25 sec
				noise_cnt := rand.Intn(90) + 10
				var wg2 sync.WaitGroup
				wg2.Add(noise_cnt)
				// we have `request_cnt` invoke, so we have `request_cnt` latency
				for i := 0; i < noise_cnt; i++ {
					go func() {
						defer wg2.Done()
						// maybe we should choose another function
						func_seq := rand.Intn(len(fnIDs))
						fnID := fnIDs[func_seq]
						ctx, _ := common.LoggerWithFields(c.Request.Context(), logrus.Fields{"fn_id": fnID})
						c.Request = c.Request.WithContext(ctx)
						err := s.handleRandomCall2(c, fnID)
						if err != nil {
							handleErrorResponse(c, err)
						}
					}()
				}
				wg2.Wait()
			}
		}()
		time.Sleep(250 * time.Millisecond)
	}

	wg.Wait()
	fmt.Println("All random noise loaded.")

}

func (s *Server) handleRandomCall2(c *gin.Context, fnID string) error {
	ctx := c.Request.Context()
	fn, err := s.lbReadAccess.GetFnByID(ctx, fnID)
	if err != nil {
		return err
	}

	app, err := s.lbReadAccess.GetAppByID(ctx, fn.AppID)
	if err != nil {
		return err
	}

	err = s.ServeFnSpike(c, app, fn)
	// if models.IsFuncError(err) || err == nil {
	// 	// report all user-directed errors and function responses from here, after submit has run.
	// 	// this is our never ending attempt to distinguish user and platform errors.
	// 	ctx, err := tag.New(ctx,
	// 		tag.Insert(whodunitKey, "user"),
	// 	)
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// 	c.Request = c.Request.WithContext(ctx)
	// }
	return err
}

func (s *Server) fnInvokeFunctionWithResult(header http.Header, req *http.Request, app *models.App, fn *models.Fn, trig *models.Trigger) (*string, error) {
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufPool.Put(buf)
	var writer ResponseBuffer

	writer = &syncResponseWriter{
		headers: header,
		status:  200,
		Buffer:  buf,
	}

	opts := getCallOptions(req, app, fn, trig, writer)

	call, err := s.agent.GetCall(opts...)
	if err != nil {
		return nil, err
	}

	// add this before submit, always tie a call id to the response at this point
	writer.Header().Add("Fn-Call-Id", call.Model().ID)

	err = s.agent.Submit(call)
	if err != nil {
		return nil, err
	}

	res := buf.String()

	fmt.Printf("result: %s\n", res)

	return &res, nil
}

func getCallOptions(req *http.Request, app *models.App, fn *models.Fn, trig *models.Trigger, rw http.ResponseWriter) []agent.CallOpt {
	var opts []agent.CallOpt
	opts = append(opts, agent.WithWriter(rw)) // XXX (reed): order matters [for now]
	opts = append(opts, agent.FromHTTPFnRequest(app, fn, req))

	if req.Header.Get("Fn-Invoke-Type") == models.TypeDetached {
		opts = append(opts, agent.InvokeDetached())
	}

	if trig != nil {
		opts = append(opts, agent.WithTrigger(trig))
	}
	return opts
}
