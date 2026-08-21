/*
 *  Copyright (c) Huawei Technologies Co., Ltd. 2025-2025. All rights reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *       http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

// Package storagefake provides an in-memory fake of Huawei storage REST APIs
// so tests can exercise the real storage client code without a real array.
package storagefake

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

const (
	restBasePath = "/deviceManager/rest"

	// FakeDeviceID is a stable fake device id returned by the login API.
	FakeDeviceID = "0000000000000000"
	// FakeToken is a stable fake iBaseToken returned by the login API.
	FakeToken = "fake-ibase-token"
	// FakeWWN is a stable fake WWN used for created LUNs.
	FakeWWN = "fake-wwn"
)

// SANLun is an in-memory LUN object.
type SANLun struct {
	ID           string
	Name         string
	WWN          string
	Capacity     string // stored in capacity units (512B sectors) as the real API does
	ParentName   string
	HasRSSObject string
}

// OceanStorSAN is a fake OceanStor SAN (LUN) storage backend served over HTTP.
type OceanStorSAN struct {
	mu sync.Mutex

	pools map[string]string // pool name -> pool ID
	luns  map[string]*SANLun

	nextLunID int
	server    *httptest.Server

	// failRules maps "METHOD /normalizedPath" -> non-zero error code. When set,
	// the fake responds with that error code instead of serving the request.
	failRules map[string]int
}

// NewOceanStorSAN creates a fake backend with the given pool registered.
func NewOceanStorSAN(poolName string) *OceanStorSAN {
	return &OceanStorSAN{
		pools:     map[string]string{poolName: "fake-pool-id"},
		luns:      make(map[string]*SANLun),
		nextLunID: 1,
		failRules: make(map[string]int),
	}
}

// Start launches the HTTP server and returns its base URL (without /deviceManager/rest).
func (s *OceanStorSAN) Start() string {
	if s.server != nil {
		return s.server.URL
	}
	s.server = httptest.NewServer(http.HandlerFunc(s.handle))
	return s.server.URL
}

// Close stops the HTTP server.
func (s *OceanStorSAN) Close() {
	if s.server != nil {
		s.server.Close()
		s.server = nil
	}
}

// URL returns the base URL, starting the server if needed.
func (s *OceanStorSAN) URL() string {
	return s.Start()
}

// LunCount returns the number of LUNs currently stored in the fake.
func (s *OceanStorSAN) LunCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.luns)
}

// SetFail injects a non-zero error code for requests of the given method and
// path prefix (e.g. method "POST", path "/lun").
func (s *OceanStorSAN) SetFail(method, path string, code int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failRules[method+" "+path] = code
}

func (s *OceanStorSAN) handle(w http.ResponseWriter, r *http.Request) {
	path := normalizePath(r.URL.Path)
	key := r.Method + " " + path

	s.mu.Lock()
	code, hasFail := s.failRules[key]
	s.mu.Unlock()
	if hasFail {
		writeResponse(w, code, nil)
		return
	}

	switch {
	case r.Method == http.MethodPost && path == "/xx/sessions":
		s.handleLogin(w, r)
	case r.Method == http.MethodDelete && path == "/sessions":
		writeResponse(w, 0, nil)
	case r.Method == http.MethodGet && path == "/system":
		s.handleSystem(w, r)
	case r.Method == http.MethodGet && path == "/storagepool":
		s.handlePools(w, r)
	case r.Method == http.MethodGet && path == "/lun":
		s.handleListLuns(w, r)
	case r.Method == http.MethodPost && path == "/lun":
		s.handleCreateLun(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/lun/"):
		s.handleGetLun(w, r, strings.TrimPrefix(path, "/lun/"))
	case r.Method == http.MethodDelete && strings.HasPrefix(path, "/lun/"):
		s.handleDeleteLun(w, r, strings.TrimPrefix(path, "/lun/"))
	default:
		writeResponse(w, 404, nil)
	}
}

func (s *OceanStorSAN) handleLogin(w http.ResponseWriter, _ *http.Request) {
	data := map[string]interface{}{
		"deviceid":   FakeDeviceID,
		"iBaseToken": FakeToken,
		"vstoreId":   "0",
		"vstoreName": "System_vStore",
	}
	writeResponse(w, 0, data)
}

func (s *OceanStorSAN) handleSystem(w http.ResponseWriter, _ *http.Request) {
	data := map[string]interface{}{
		"PRODUCTVERSION": "V500R007C00", // V5 -> keeps the plain OceanStorClient (no V6 client switch)
		"PRODUCTMODE":    "5500",
		"pointRelease":   "V500R007C00",
		"wwn":            FakeWWN,
		"NAME":           "fake-array",
	}
	writeResponse(w, 0, data)
}

func (s *OceanStorSAN) handlePools(w http.ResponseWriter, r *http.Request) {
	name := filterName(r.URL.Query().Get("filter"))
	if name != "" {
		if id, ok := s.lookupPool(name); ok {
			writeResponse(w, 0, []interface{}{
				map[string]interface{}{
					"ID":            id,
					"NAME":          name,
					"TOTALCAPACITY": "1048576",
					"FREECAPACITY":  "1048576",
					"USEDCAPACITY":  "0",
				},
			})
			return
		}
	}
	writeResponse(w, 0, []interface{}{})
}

func (s *OceanStorSAN) handleListLuns(w http.ResponseWriter, r *http.Request) {
	name := filterName(r.URL.Query().Get("filter"))
	if name == "" {
		writeResponse(w, 0, []interface{}{})
		return
	}

	s.mu.Lock()
	var result []interface{}
	for _, lun := range s.luns {
		if lun.Name == name {
			result = append(result, lunToMap(lun))
		}
	}
	s.mu.Unlock()

	writeResponse(w, 0, result)
}

func (s *OceanStorSAN) handleGetLun(w http.ResponseWriter, _ *http.Request, id string) {
	s.mu.Lock()
	lun, ok := s.luns[id]
	s.mu.Unlock()
	if !ok {
		writeResponse(w, 1073751811, nil) // lunNotExist
		return
	}
	writeResponse(w, 0, lunToMap(lun))
}

func (s *OceanStorSAN) handleCreateLun(w http.ResponseWriter, r *http.Request) {
	var params map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		writeResponse(w, 1073807377, nil) // parameterIncorrect
		return
	}

	name, _ := params["NAME"].(string)
	capacityF, _ := params["CAPACITY"].(float64)
	capacity := int64(capacityF)
	parent, _ := params["PARENTID"]

	s.mu.Lock()
	defer s.mu.Unlock()

	id := fmt.Sprintf("fake-lun-%03d", s.nextLunID)
	s.nextLunID++

	lun := &SANLun{
		ID:           id,
		Name:         name,
		WWN:          FakeWWN,
		Capacity:     fmt.Sprintf("%d", capacity),
		ParentName:   fmt.Sprintf("%v", parent),
		HasRSSObject: "{}",
	}
	s.luns[id] = lun

	writeResponse(w, 0, lunToMap(lun))
}

func (s *OceanStorSAN) handleDeleteLun(w http.ResponseWriter, _ *http.Request, id string) {
	s.mu.Lock()
	_, ok := s.luns[id]
	if ok {
		delete(s.luns, id)
	}
	s.mu.Unlock()
	writeResponse(w, 0, nil)
}

func (s *OceanStorSAN) lookupPool(name string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.pools[name]
	return id, ok
}

func lunToMap(lun *SANLun) map[string]interface{} {
	return map[string]interface{}{
		"ID":                 lun.ID,
		"NAME":               lun.Name,
		"WWN":                lun.WWN,
		"CAPACITY":           lun.Capacity,
		"PARENTNAME":         lun.ParentName,
		"HASRSSOBJECT":       lun.HasRSSObject,
		"EXPOSEDTOINITIATOR": "false",
		"vstoreName":         "System_vStore",
	}
}

func filterName(filter string) string {
	const prefix = "NAME::"
	if strings.HasPrefix(filter, prefix) {
		return strings.TrimPrefix(filter, prefix)
	}
	return ""
}

// normalizePath strips the /deviceManager/rest prefix and any optional device-id
// segment so the fake can be served both before and after login.
func normalizePath(raw string) string {
	p := raw
	if idx := strings.Index(p, restBasePath); idx >= 0 {
		p = p[idx+len(restBasePath):]
	}
	if p == "" {
		p = "/"
	}
	if p[0] != '/' {
		p = "/" + p
	}

	trimmed := strings.TrimRight(p, "/")
	first := strings.SplitN(trimmed, "/", 3)
	// First segment is the device id when it is not a known resource root.
	if len(first) > 1 && !knownResourceRoot(first[1]) {
		trimmed = "/" + strings.Join(first[2:], "/")
	}

	if trimmed == "/system" || trimmed == "/system/" {
		return "/system"
	}
	return trimmed
}

func knownResourceRoot(seg string) bool {
	switch seg {
	case "xx", "sessions", "system", "storagepool", "lun":
		return true
	}
	return false
}

func writeResponse(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	resp := map[string]interface{}{
		"error": map[string]interface{}{
			"code":        code,
			"description": "",
		},
		"data": data,
	}
	_ = json.NewEncoder(w).Encode(resp)
}
