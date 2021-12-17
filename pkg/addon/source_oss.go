package addon

import (
	"encoding/xml"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/pkg/errors"
)

var _ AsyncReader = &ossReader{}

// ListBucketResult describe a file list from OSS
type ListBucketResult struct {
	Files []File `xml:"Contents"`
	Count int    `xml:"KeyCount"`
}

// File is for oss xml parse
type File struct {
	Name         string    `xml:"Key"`
	Size         int       `xml:"Size"`
	LastModified time.Time `xml:"LastModified"`
	Type         string    `xml:"Type"`
	StorageClass string    `xml:"StorageClass"`
}

type ossReader struct {
	bucketEndPoint string
	path           string
	client         *resty.Client
}

// OSSItem is Item implement for OSS
type OSSItem struct {
	tp   string
	path string
	name string
}

// GetType from OSSItem
func (i OSSItem) GetType() string {
	return i.tp
}

// GetPath from OSSItem
func (i OSSItem) GetPath() string {
	return i.path
}

// GetName from OSSItem
func (i OSSItem) GetName() string {
	return i.name
}

// ReadFile read file content from OSS bucket, path is relative to oss bucket and sub-path in reader
func (o *ossReader) ReadFile(relativePath string) (content string, err error) {
	resp, err := o.client.R().Get(fmt.Sprintf(singleOSSFileTmpl, o.bucketEndPoint, path.Join(o.path, relativePath)))
	if err != nil {
		return "", err
	}
	return string(resp.Body()), nil
}

// ListAddonMeta list object from OSS and convert it to UIData metadata
func (o *ossReader) ListAddonMeta() (subItem map[string]SourceMeta, err error) {
	resp, err := o.client.R().Get(fmt.Sprintf(listOSSFileTmpl, o.bucketEndPoint, o.path))
	if err != nil {
		return nil, errors.Wrapf(err, "fail to read path %s", o.path)
	}

	list := ListBucketResult{}
	err = xml.Unmarshal(resp.Body(), &list)
	if err != nil {
		return nil, err
	}
	list = filterEmptyObj(list)
	addons := o.convertOSSFiles2Addons(list.Files)
	return addons, nil
}

// convertOSSFiles2Addons convert OSS list result to map of addon meta information
func (o ossReader) convertOSSFiles2Addons(files []File) map[string]SourceMeta {
	addonMetas := make(map[string]SourceMeta)
	pathBuckets := make(map[string][]Item)
	fPaths := make(map[string][]string)
	actualFiles := make([]File, 0)
	// first traversal to confirm addon and initialize addonMetas
	for _, f := range files {
		fPath := trimAndSplitPath(f.Name, o.path)
		if len(fPath) < 2 || f.Size == 0 {
			// this is a file or directory in root, remove it
			continue
		}
		fPaths[f.Name] = fPath
		actualFiles = append(actualFiles, f)
		var addonName = fPath[0]
		if len(fPath) == 2 && fPath[1] == MetadataFileName {
			addonMetas[addonName] = SourceMeta{Name: addonName}
			pathBuckets[addonName] = make([]Item, 0)
		}
	}
	for _, f := range actualFiles {
		fPath := fPaths[f.Name]
		addonName := fPath[0]
		pathList := pathBuckets[addonName]
		pathList = append(pathList, &OSSItem{
			path: path.Join(fPath...),
			tp:   FileType,
			name: fPath[len(fPath)-1],
		})
		pathBuckets[addonName] = pathList
	}
	var addonList = make(map[string]SourceMeta)
	for k, v := range addonMetas {
		items := pathBuckets[k]
		sort.Slice(items, func(i, j int) bool {
			return items[i].GetPath() < items[j].GetPath()
		})
		v.Items = pathBuckets[k]
		addonList[k] = v
	}
	return addonList
}

func trimAndSplitPath(absPath string, path2Bucket string) []string {
	const slash = "/"
	var p = absPath
	if path2Bucket != "" {
		p = strings.TrimPrefix(p, path2Bucket)
		p = strings.TrimPrefix(p, "/")
	}
	return strings.Split(p, slash)
}

func (o *ossReader) RelativePath(item Item) string {
	return item.GetPath()
}

// OSSAddonSource is UIData source from alibaba cloud OSS style source
type OSSAddonSource struct {
	Endpoint string `json:"endpoint" validate:"required"`
	Bucket   string `json:"bucket"`
	Path     string `json:"path"`
}

// GetUIMeta from OSS Addon data Source
func (o *OSSAddonSource) GetUIMeta(meta *SourceMeta, opt ListOptions) (*UIData, error) {
	reader, err := NewAsyncReader(o.Endpoint, o.Bucket, o.Path, "", ossType)
	if err != nil {
		return nil, err
	}
	addon, err := GetUIMetaFromReader(reader, meta, opt)
	if err != nil {
		return nil, err
	}
	return addon, nil
}

// ListRegistryMeta will list registry add meta for cache
func (o *OSSAddonSource) ListRegistryMeta() (map[string]SourceMeta, error) {
	reader, err := NewAsyncReader(o.Endpoint, o.Bucket, o.Path, "", ossType)
	if err != nil {
		return nil, err
	}
	return reader.ListAddonMeta()
}

// ListUIData from OSSAddonSource
func (o *OSSAddonSource) ListUIData(registryMeta map[string]SourceMeta, opt ListOptions) ([]*UIData, error) {
	reader, err := NewAsyncReader(o.Endpoint, o.Bucket, o.Path, "", ossType)
	if err != nil {
		return nil, err
	}
	return GetAddonUIMetaFromReader(reader, registryMeta, opt)
}

// GetInstallPackage will get install package for addon from OSS
func (o *OSSAddonSource) GetInstallPackage(meta *SourceMeta, uiMeta *UIData) (*InstallPackage, error) {
	reader, err := NewAsyncReader(o.Endpoint, o.Bucket, o.Path, "", ossType)
	if err != nil {
		return nil, err
	}
	return GetInstallPackageFromReader(reader, meta, uiMeta)
}

func filterEmptyObj(list ListBucketResult) ListBucketResult {
	var actualFiles []File
	for _, f := range list.Files {
		if f.Size > 0 {
			actualFiles = append(actualFiles, f)
		}
	}
	return ListBucketResult{
		Files: actualFiles,
		Count: len(actualFiles),
	}
}
