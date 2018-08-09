// Copyright 2018 Google Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package githubfs

import (
	"io/ioutil"
	"os"
	"testing"
	"time"

	testServer "github.com/soumya92/barista/testing/httpserver"
	"github.com/stretchr/testify/assert"
)

func TestName(t *testing.T) {
	assert.Contains(t, New().Name(), "GitHubFS")
}

func TestFs(t *testing.T) {
	ts := testServer.New()
	defer ts.Close()
	root = ts.URL

	fs := New()

	_, err := fs.Open("/code/500")
	assert.Error(t, err)
	_, err = fs.OpenFile("/redir", 0, 0444)
	assert.Error(t, err)
	_, err = fs.Stat("/code/403")
	assert.Error(t, err)

	info, err := fs.Stat("/modtime/1382140800")
	assert.NoError(t, err)
	modTime := time.Date(2013, time.October, 19, 0, 0, 0, 0, time.UTC)
	assert.WithinDuration(t, modTime, info.ModTime(), time.Minute)

	f, err := fs.Open("/basic/empty")
	assert.NoError(t, err)
	contents, err := ioutil.ReadAll(f)
	assert.NoError(t, err)
	assert.Equal(t, []byte{}, contents)

	f, err = fs.OpenFile("/basic/foo", os.O_RDONLY, 0600)
	assert.NoError(t, err)
	contents, err = ioutil.ReadAll(f)
	assert.NoError(t, err)
	assert.Equal(t, "bar", string(contents))
}