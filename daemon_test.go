// Copyright 2016 Mender Software AS
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.
package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func Test_loadConfig_noConfigFile_returnsDefaultConfig(t *testing.T) {
	daemon := menderDaemon{}
	daemon.LoadConfig("non_existing")

	config := daemonConfigType{
		defaultServerpollInterval,
		defaultServerAddress,
		defaultDeviceID,
	}

	if !reflect.DeepEqual(daemon.config, config) {
		t.FailNow()
	}
}

func Test_loadConfigFromServerFile_ServerFileExists(t *testing.T) {
	if server := getMenderServer("non-existing-file.server"); !strings.Contains(server, defaultServerAddress) {
		t.Fatal("Expecting default mender server, received " + server)
	}

	// test if file is parsed correctly
	srvFile, err := os.Create("mender.server")
	if err != nil {
		t.Fail()
	}

	defer os.Remove("mender.server")

	if _, err := srvFile.WriteString("https://testserver"); err != nil {
		t.Fail()
	}
	if server := getMenderServer("mender.server"); strings.Compare("https://testserver", server) != 0 {
		t.Fatal("Unexpected mender server name, received " + server)
	}
}

type fakeDevice struct {
	retReboot        error
	retInstallUpdate error
	retEnablePart    error
	retCommit        error
}

func (f fakeDevice) Reboot() error {
	return f.retReboot
}

func (f fakeDevice) InstallUpdate(io.ReadCloser, int64) error {
	return f.retInstallUpdate
}

func (f fakeDevice) EnableUpdatedPartition() error {
	return f.retEnablePart
}

func (f fakeDevice) CommitUpdate() error {
	return f.retCommit
}

type fakeUpdater struct {
	GetScheduledUpdateReturnIface interface{}
	GetScheduledUpdateReturnError error
	fetchUpdateReturnReadCloser   io.ReadCloser
	fetchUpdateReturnSize         int64
	fetchUpdateReturnError        error
}

func (f fakeUpdater) GetScheduledUpdate(process RequestProcessingFunc,
	url string) (interface{}, error) {
	return f.GetScheduledUpdateReturnIface, f.GetScheduledUpdateReturnError
}
func (f fakeUpdater) FetchUpdate(url string) (io.ReadCloser, int64, error) {
	return f.fetchUpdateReturnReadCloser, f.fetchUpdateReturnSize, f.fetchUpdateReturnError
}

func fakeProcessUpdate(response *http.Response) (interface{}, error) {
	return nil, nil
}

func Test_performUpdate_errorAskingForUpdate_returnsError(t *testing.T) {
	updater := fakeUpdater{}
	updater.GetScheduledUpdateReturnError = errors.New("")
	device := fakeDevice{}

	if _, err := performUpdate(updater, device, fakeProcessUpdate, ""); err == nil {
		t.FailNow()
	}
}

func Test_performUpdate_askingForUpdateReturnsEmpty_returnsNilAndFalse(t *testing.T) {
	updater := fakeUpdater{}
	device := fakeDevice{}

	if upd, err := performUpdate(updater, device, fakeProcessUpdate, ""); err != nil || upd == true {
		t.Fatal(upd)
	}
}

func Test_performUpdate_updateFetchError_returnsError(t *testing.T) {
	updater := fakeUpdater{}
	updater.GetScheduledUpdateReturnIface = new(UpdateResponse)
	updater.fetchUpdateReturnError = errors.New("")
	device := fakeDevice{}

	if upd, err := performUpdate(updater, device, fakeProcessUpdate, ""); err == nil || upd == true {
		t.FailNow()
	}
}

func Test_performUpdate_updateFetchOK_returnsSuccess(t *testing.T) {
	updater := fakeUpdater{}
	updater.GetScheduledUpdateReturnIface = new(UpdateResponse)
	device := fakeDevice{}

	if upd, err := performUpdate(updater, device, fakeProcessUpdate, ""); err != nil || upd == false {
		t.FailNow()
	}
}

func Test_performUpdate_updateFetchOKInstallError_returnsError(t *testing.T) {
	updater := fakeUpdater{}
	updater.GetScheduledUpdateReturnIface = new(UpdateResponse)
	device := fakeDevice{}
	device.retInstallUpdate = errors.New("")

	if upd, err := performUpdate(updater, device, fakeProcessUpdate, ""); err == nil || upd == true {
		t.FailNow()
	}
}

func Test_performUpdate_updateFetchOKEnableError_returnsError(t *testing.T) {
	updater := fakeUpdater{}
	updater.GetScheduledUpdateReturnIface = new(UpdateResponse)
	device := fakeDevice{}
	device.retEnablePart = errors.New("")

	if upd, err := performUpdate(updater, device, fakeProcessUpdate, ""); err == nil || upd == true {
		t.FailNow()
	}
}

func Test_checkPeriodicDaemonUpdate_haveServerAndCorrectResponse_FetchesUpdate(t *testing.T) {
	reqHandlingCnt := 0
	pollInterval := time.Duration(100) * time.Millisecond

	// Test server that always responds with 200 code, and specific payload
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, correctUpdateResponse)
		reqHandlingCnt++
	}))
	defer ts.Close()

	client := NewClient(authCmdLineArgsType{ts.URL, "client.crt", "client.key", "server.crt"})
	device := NewDevice(nil, nil, "")
	daemon := NewDaemon(client, device)
	daemon.config = daemonConfigType{serverpollInterval: pollInterval, server: ts.URL}

	go daemon.Run()

	timespolled := 5
	time.Sleep(time.Duration(timespolled) * pollInterval)
	daemon.StopDaemon()

	if reqHandlingCnt < (timespolled - 1) {
		t.Fatal("Expected to receive at least ", timespolled-1, " requests - ", reqHandlingCnt, " received")
	}
}
