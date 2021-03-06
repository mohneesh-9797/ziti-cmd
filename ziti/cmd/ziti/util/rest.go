/*
	Copyright NetFoundry, Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package util

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"github.com/Jeffail/gabs"
	"github.com/blang/semver"
	"github.com/netfoundry/ziti-cmd/common/version"
	cmdhelper "github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/cmd/helpers"
	c "github.com/netfoundry/ziti-cmd/ziti/cmd/ziti/constants"
	"github.com/netfoundry/ziti-foundation/common/constants"
	"gopkg.in/resty.v1"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/template"
	"time"
)

// Download a file from the given URL
func DownloadFile(filepath string, url string) (err error) {
	// Create the file
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Writer the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	// make it executable
	err = os.Chmod(filepath, 0755)

	if err != nil {
		return err
	}
	return nil
}

// Use a 2-second timeout with a retry count of 5
func newClient() *resty.Client {
	return resty.
		New().
		SetTimeout(time.Duration(2 * time.Second)).
		SetRetryCount(5).
		SetRedirectPolicy(resty.FlexibleRedirectPolicy(15))
}

func getRequest(verbose bool) *resty.Request {
	return newClient().
		SetDebug(verbose).
		R()
}

// DownloadArtifactoryFile will download a file from the given Artifactory URL
func DownloadArtifactoryFile(url string, filepath string) (err error) {
	fullUrl := "https://netfoundry.jfrog.io/netfoundry/" + url
	resp, err := getRequest(false).
		SetHeader("X-JFrog-Art-Api", cmdhelper.JFrogAPIKey()).
		SetOutput(filepath).
		Get(fullUrl)

	if err != nil {
		return fmt.Errorf("unable to download '%s', %s", url, err)
	}

	if resp.IsError() {
		return fmt.Errorf("unable to download file, error HTTP status code [%d] returned for url [%s]", resp.StatusCode(), fullUrl)
	}

	return nil
}

// Used to parse the 'get-object-tagging' response
type Data struct {
	TagSet []struct {
		Value string `json:"Value"`
		Key   string `json:"Key"`
	}
}

// Used to parse the '/api/versions' response from Artifactory
type ArtifactoryVersionsData struct {
	Version   string `json:"version"`
	Artifacts []struct {
	}
}

func GetLatestVersionFromArtifactory(verbose bool, staging bool, branch string, appName string) (semver.Version, error) {
	resp, err := getRequest(verbose).
		SetQueryParams(map[string]string{
			//   "key": "value",
		}).
		SetHeader("X-JFrog-Art-Api", cmdhelper.JFrogAPIKey()).
		SetResult(&ArtifactoryVersionsData{}).
		Get("https://netfoundry.jfrog.io/netfoundry/api/versions/" + GetArtifactoryPath(staging, appName, branch))

	if err != nil {
		return semver.Version{}, fmt.Errorf("unable to get latest version for '%s'; %s", appName, err)
	}

	if resp.StatusCode() == http.StatusNotFound {
		return semver.Version{}, fmt.Errorf("unable to get latest version for '%s' on branch '%s'; Not Found", appName, branch)
	}
	if resp.StatusCode() != http.StatusOK {
		return semver.Version{}, fmt.Errorf("unable to get latest version for '%s' on branch '%s'; %s", appName, branch, resp.Status())
	}

	result := (*resp.Result().(*ArtifactoryVersionsData))

	return semver.Make(strings.TrimPrefix(result.Version, "v"))
}

// Used to parse the '/api/search/aql' response from Artifactory
type AQLResult struct {
	Repo       string
	Path       string
	Name       string
	Type       string
	Size       int
	Created    string
	CreatedBy  string
	Modified   string
	ModifiedBy string
	Updated    string
	Properties []struct {
		Key   string
		Value string
	}
}
type ArtifactoryAQLData struct {
	Results []AQLResult
}

type AQLVars struct {
	SemverMajor    string
	SemverMaxMinor string
	SemverMinMinor string
	App            string
	Arch           string
	OS             string
}

func GetLatestSemanticVersionFromArtifactory(verbose bool, staging bool, branch string, appName string, versionWildcard string) (semver.Version, error) {
	sv := strings.Split(versionWildcard, ".")
	minor, err := strconv.Atoi(sv[1])
	if err != nil {
		panic(err)
	}
	maxMinor := minor + 1
	aqlVars := AQLVars{sv[0], strconv.Itoa(maxMinor), sv[1], appName, runtime.GOARCH, runtime.GOOS}
	tpl, err := template.New("aql").Parse("items.find( { \"@build.number\":{\"$lt\":\"{{ .SemverMajor}}.{{ .SemverMaxMinor}}.*\"}, \"@build.number\":{\"$gt\":\"{{ .SemverMajor}}.{{ .SemverMinMinor}}.*\"}, \"repo\":{\"$match\":\"ziti-release\"}, \"path\":{\"$match\":\"{{ .App}}/{{ .Arch}}/{{ .OS}}/*\"} } ).include(\"@build.number\") ")
	if err != nil {
		panic(err)
	}
	var body bytes.Buffer
	err = tpl.Execute(&body, aqlVars)
	if err != nil {
		panic(err)
	}
	resp, err := getRequest(verbose).
		SetHeader("X-JFrog-Art-Api", cmdhelper.JFrogAPIKey()).
		SetHeader("Content-Type", "text/plain").
		SetBody(body.String()).
		SetResult(&ArtifactoryAQLData{}).
		Post("https://netfoundry.jfrog.io/netfoundry/api/search/aql")

	if err != nil {
		return semver.Version{}, fmt.Errorf("unable to get latest version for '%s'; %s", appName, err)
	}

	if resp.StatusCode() == http.StatusNotFound {
		return semver.Version{}, fmt.Errorf("unable to get latest version for '%s' on branch '%s'; Not Found", appName, branch)
	}
	if resp.StatusCode() != http.StatusOK {
		return semver.Version{}, fmt.Errorf("unable to get latest version for '%s' on branch '%s'; %s", appName, branch, resp.Status())
	}

	aqlData := (*resp.Result().(*ArtifactoryAQLData))

	latestSemVer, _ := semver.Make("0.0.0")

	for _, result := range aqlData.Results {
		sv, err := semver.Make(result.Properties[0].Value)
		if err != nil {
			panic(err)
		}
		if sv.GT(latestSemVer) {
			latestSemVer = sv
		}
	}

	return latestSemVer, nil
}

func GetLatestTerraformProviderVersionFromArtifactory(branch string, provider string) (semver.Version, error) {
	repoUrl := "https://netfoundry.jfrog.io/netfoundry/api/versions/" + GetTerraformProviderArtifactoryPath(provider, branch)
	resp, err := getRequest(false).
		SetQueryParams(map[string]string{
			//   "key": "value",
		}).
		SetHeader("X-JFrog-Art-Api", cmdhelper.JFrogAPIKey()).
		SetResult(&ArtifactoryVersionsData{}).
		Get(repoUrl)

	if err != nil {
		return semver.Version{}, fmt.Errorf("unable to get latest version for '%s' on repo url %s; %s", provider, repoUrl, err)
	}

	if resp.StatusCode() == http.StatusNotFound {
		return semver.Version{}, fmt.Errorf("unable to get latest version for '%s' on branch '%s' on repo url %s; Not Found", provider, branch, repoUrl)
	}
	if resp.StatusCode() != http.StatusOK {
		return semver.Version{}, fmt.Errorf("unable to get latest version for '%s' on branch '%s' on repo url %s; %s", provider, branch, repoUrl, resp.Status())
	}

	result := (*resp.Result().(*ArtifactoryVersionsData))

	return semver.Make(strings.TrimPrefix(result.Version, "v"))
}

// Used to parse the '/api/storage' response from Artifactory
type ArtifactoryStorageChildrenData struct {
	URI    string `json:"uri"`
	Folder bool   `json:"folder"`
}
type ArtifactoryStorageData struct {
	Repo         string                           `json:"repo"`
	Path         string                           `json:"path"`
	Created      string                           `json:"created"`
	LastModified string                           `json:"lastModified"`
	LastUpdated  string                           `json:"lastUpdated"`
	Children     []ArtifactoryStorageChildrenData `json:"children"`
	URI          string                           `json:"uri"`
}

func GetCurrentSnapshotListFromArtifactory(verbose bool) ([]ArtifactoryStorageChildrenData, error) {
	resp, err := getRequest(verbose).
		SetHeader("X-JFrog-Art-Api", cmdhelper.JFrogAPIKey()).
		SetResult(&ArtifactoryStorageData{}).
		Get("https://netfoundry.jfrog.io/netfoundry/api/storage/ziti-snapshot/")

	if err != nil {
		return nil, fmt.Errorf("unable to get list of branches; %s", err)
	}

	result := (*resp.Result().(*ArtifactoryStorageData))

	return result.Children, nil
}

func GetArtifactoryPath(staging bool, appName string, branch string) string {
	if branch == "" {
		branch = version.GetBranch()
	}

	arch := runtime.GOARCH
	os := runtime.GOOS

	var path string
	if staging {
		path = "ziti-staging/"
	} else if branch == "master" {
		path = "ziti-release/"
	} else {
		path = "ziti-snapshot/" + branch + "/"
	}
	// Special-case the source-repo when dealing with ziti-prox-c
	if branch == "master" && appName == c.ZITI_PROX_C {
		path = "ziti-staging/"
	}

	path += appName + "/" + arch + "/" + os

	return path
}

func GetTerraformProviderArtifactoryPath(provider string, branch string) string {
	if branch == "" {
		branch = "master"
	}
	var path string
	if branch == "master" {
		path = c.TERRAFORM_PROVIDER_PREFIX + provider + "-release/"
	} else {
		path = c.TERRAFORM_PROVIDER_PREFIX + provider + "-snapshot/" + branch + "/"
	}
	path += c.TERRAFORM_PROVIDER_PREFIX + provider + "/" + version.GetArchitecture() + "/" + version.GetOS()

	return path
}

// untargz a tarball to a target, from
// http://blog.ralch.com/tutorial/golang-working-with-tar-and-gzip
func UnTargz(tarball, target string, onlyFiles []string) error {
	zreader, err := os.Open(tarball)
	if err != nil {
		return err
	}
	defer zreader.Close()

	reader, err := gzip.NewReader(zreader)
	defer func() {
		_ = reader.Close()
	}()

	if err != nil {
		panic(err)
	}

	tarReader := tar.NewReader(reader)

	for {
		inkey := false
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		for _, value := range onlyFiles {
			if value == "*" || value == path.Base(header.Name) {
				inkey = true
				break
			}
		}

		if !inkey {
			continue
		}

		path := filepath.Join(target, path.Base(header.Name))
		info := header.FileInfo()
		if info.IsDir() {
			if err = os.MkdirAll(path, info.Mode()); err != nil {
				return err
			}
			continue
		}

		file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(file, tarReader)
		if err != nil {
			return err
		}
	}
	return nil
}

// EdgeControllerLogin will authenticate to the given Edge Controller
func EdgeControllerLogin(url string, cert string, authentication string, out io.Writer, logJSON bool) (*gabs.Container, error) {
	client := newClient()

	if cert != "" {
		client.SetRootCertificate(cert)
	}

	resp, err := client.
		R().
		SetQueryParam("method", "password").
		SetHeader("Content-Type", "application/json").
		SetBody(authentication).
		Post(url + "/authenticate")

	if err != nil {
		return nil, fmt.Errorf("unable to authentiate to %v. Error: %v", url, err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("unable to authenticate to %v. Status code: %v, Server returned: %v", url, resp.Status(), resp.String())
	}

	if logJSON {
		outputJson(out, resp.Body())
	}

	jsonParsed, err := gabs.ParseJSON(resp.Body())
	if err != nil {
		return nil, fmt.Errorf("unable to parse response from %v. Server returned: %v", url, resp.String())
	}

	return jsonParsed, nil
}

func outputJson(out io.Writer, data []byte) {
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, data, "", "    "); err == nil {
		if _, err := fmt.Fprint(out, prettyJSON.String()); err != nil {
			panic(err)
		}
	} else {
		if _, err := fmt.Fprint(out, data); err != nil {
			panic(err)
		}
	}
}

// EdgeControllerListSubEntities will list entities of the given type in the given Edge Controller
func EdgeControllerListSubEntities(entityType, subType, entityId string, filter string, logJSON bool, out io.Writer) (*gabs.Container, error) {
	params := url.Values{}
	if filter != "" {
		params.Add("filter", filter)
	}
	return EdgeControllerList(entityType+"/"+entityId+"/"+subType, params, logJSON, out)
}

// EdgeControllerList will list entities of the given type in the given Edge Controller
func EdgeControllerList(path string, params url.Values, logJSON bool, out io.Writer) (*gabs.Container, error) {
	session := &Session{}
	if err := session.Load(); err != nil {
		return nil, err
	}

	client := newClient()

	if session.GetCert() != "" {
		client.SetRootCertificate(session.GetCert())
	}

	queryUrl := session.GetBaseUrl() + "/" + path

	if len(params) > 0 {
		queryUrl += "?" + params.Encode()
	}

	resp, err := client.
		R().
		SetHeader("Content-Type", "application/json").
		SetHeader(constants.ZitiSession, session.GetToken()).
		Get(queryUrl)

	if err != nil {
		return nil, fmt.Errorf("unable to list entities at %v in Ziti Edge Controller at %v. Error: %v", queryUrl, session.GetBaseUrl(), err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("error listing %v in Ziti Edge Controller. Status code: %v, Server returned: %v",
			queryUrl, resp.Status(), resp.String())
	}

	if logJSON {
		outputJson(out, resp.Body())
	}

	jsonParsed, err := gabs.ParseJSON(resp.Body())

	if err != nil {
		return nil, fmt.Errorf("unable to parse response from %v. Server returned: %v", queryUrl, resp.String())
	}

	return jsonParsed, nil
}

// EdgeControllerCreate will create entities of the given type in the given Edge Controller
func EdgeControllerCreate(entityType string, body string, out io.Writer, logJSON bool) (*gabs.Container, error) {
	session := &Session{}
	if err := session.Load(); err != nil {
		return nil, err
	}

	client := newClient()

	if session.Cert != "" {
		client.SetRootCertificate(session.Cert)
	}
	resp, err := client.
		R().
		SetHeader("Content-Type", "application/json").
		SetHeader(constants.ZitiSession, session.Token).
		SetBody(body).
		Post(session.Host + "/" + entityType)

	if err != nil {
		return nil, fmt.Errorf("unable to create %v instance in Ziti Edge Controller at %v. Error: %v", entityType, session.Host, err)
	}

	if resp.StatusCode() != http.StatusCreated {
		return nil, fmt.Errorf("error creating %v instance in Ziti Edge Controller at %v. Status code: %v, Server returned: %v",
			entityType, session.Host, resp.Status(), resp.String())
	}

	if logJSON {
		outputJson(out, resp.Body())
	}

	jsonParsed, err := gabs.ParseJSON(resp.Body())

	if err != nil {
		return nil, fmt.Errorf("unable to parse response from %v. Server returned: %v", session.Host, resp.String())
	}

	return jsonParsed, nil
}

// EdgeControllerDelete will delete entities of the given type in the given Edge Controller
func EdgeControllerDelete(entityType string, id string, out io.Writer, logJSON bool) (*gabs.Container, error) {
	session := &Session{}
	if err := session.Load(); err != nil {
		return nil, err
	}

	client := newClient()

	if session.Cert != "" {
		client.SetRootCertificate(session.Cert)
	}
	entityPath := entityType + "/" + id
	fullUrl := session.Host + "/" + entityPath

	resp, err := client.
		R().
		SetHeader("Content-Type", "application/json").
		SetHeader(constants.ZitiSession, session.Token).
		Delete(fullUrl)

	if err != nil {
		return nil, fmt.Errorf("unable to delete %v instance in Ziti Edge Controller at %v. Error: %v", entityPath, session.Host, err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("error deleting %v instance in Ziti Edge Controller at %v. Status code: %v, Server returned: %v",
			entityPath, session.Host, resp.Status(), resp.String())
	}

	if logJSON {
		outputJson(out, resp.Body())
	}

	jsonParsed, err := gabs.ParseJSON(resp.Body())

	if err != nil {
		return nil, fmt.Errorf("unable to parse response from %v. Server returned: %v", session.Host, resp.String())
	}

	return jsonParsed, nil
}

// EdgeControllerUpdate will update entities of the given type in the given Edge Controller
func EdgeControllerUpdate(entityType string, body string, out io.Writer, put bool, logJSON bool) (*gabs.Container, error) {
	session := &Session{}
	if err := session.Load(); err != nil {
		return nil, err
	}

	client := newClient()

	if session.Cert != "" {
		client.SetRootCertificate(session.Cert)
	}

	request := client.
		R().
		SetHeader("Content-Type", "application/json").
		SetHeader(constants.ZitiSession, session.Token).
		SetBody(body)

	var err error
	var resp *resty.Response
	if put {
		resp, err = request.Put(session.Host + "/" + entityType)
	} else {
		resp, err = request.Patch(session.Host + "/" + entityType)
	}

	if err != nil {
		return nil, fmt.Errorf("unable to update %v instance in Ziti Edge Controller at %v. Error: %v", entityType, session.Host, err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("error creating %v instance in Ziti Edge Controller at %v. Status code: %v, Server returned: %v",
			entityType, session.Host, resp.Status(), resp.String())
	}

	if logJSON {
		outputJson(out, resp.Body())
	}

	jsonParsed, err := gabs.ParseJSON(resp.Body())

	if err != nil {
		return nil, fmt.Errorf("unable to parse response from %v. Server returned: %v", session.Host, resp.String())
	}

	return jsonParsed, nil
}

// EdgeControllerVerify will create entities of the given type in the given Edge Controller
func EdgeControllerVerify(entityType, id, body string, out io.Writer, logJSON bool) error {
	session := &Session{}
	if err := session.Load(); err != nil {
		return err
	}

	client := newClient()

	if session.Cert != "" {
		client.SetRootCertificate(session.Cert)
	}
	resp, err := client.
		R().
		SetHeader("Content-Type", "application/json").
		SetHeader(constants.ZitiSession, session.Token).
		SetBody(body).
		Post(session.Host + "/" + entityType + "/" + id + "/verify")

	if err != nil {
		return fmt.Errorf("unable to verify %v instance [%s] in Ziti Edge Controller at %v. Error: %v", entityType, id, session.Host, err)
	}

	if resp.StatusCode() != http.StatusOK {
		return fmt.Errorf("error verifying %v instance (%v) in Ziti Edge Controller at %v. Status code: %v, Server returned: %v",
			entityType, id, session.Host, resp.Status(), resp.String())
	}

	if logJSON {
		outputJson(out, resp.Body())
	}

	return nil
}

func EdgeControllerRequest(entityType string, out io.Writer, logJSON bool, doRequest func(*resty.Request, string) (*resty.Response, error)) (*gabs.Container, error) {
	session := &Session{}
	if err := session.Load(); err != nil {
		return nil, err
	}

	client := newClient()

	if session.Cert != "" {
		client.SetRootCertificate(session.Cert)
	}

	request := client.
		R().
		SetHeader("Content-Type", "application/json").
		SetHeader(constants.ZitiSession, session.Token)

	resp, err := doRequest(request, session.Host+"/"+entityType)

	if err != nil {
		return nil, fmt.Errorf("unable to [%s] %v instance in Ziti Edge Controller at %v. Error: %v", request.Method, entityType, session.Host, err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("error performing request [%s] %v instance in Ziti Edge Controller at %v. Status code: %v, Server returned: %v",
			request.Method, entityType, session.Host, resp.Status(), resp.String())
	}

	if logJSON {
		outputJson(out, resp.Body())
	}

	jsonParsed, err := gabs.ParseJSON(resp.Body())

	if err != nil {
		return nil, fmt.Errorf("unable to parse response from %v. Server returned: %v", session.Host, resp.String())
	}

	return jsonParsed, nil

}

//util.EdgeControllerRequest(session.Host, session.Cert, session.Token, entityType, options.Out, options.OutputJSONResponse, func(request *resty.Request) error {
