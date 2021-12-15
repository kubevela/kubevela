package addon

import (
	"encoding/xml"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/pkg/errors"
)

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

// ReadFile read file content from OSS bucket
func (o *ossReader) ReadFile(relativePath string) (content string, err error) {
	resp, err := o.client.R().Get(fmt.Sprintf(singleOSSFileTmpl, o.bucketEndPoint, relativePath))
	if err != nil {
		return "", err
	}
	return string(resp.Body()), nil
}

// ListAddonMeta list object from OSS and convert it to UIData metadata
func (o *ossReader) ListAddonMeta(readPath string) (subItem map[string]SourceMeta, err error) {
	if readPath == "." {
		readPath = ""
	}
	bucketPath := pathWithParent(readPath, o.path)
	resp, err := o.client.R().Get(fmt.Sprintf(listOSSFileTmpl, o.bucketEndPoint, bucketPath))
	if err != nil {
		return nil, errors.Wrapf(err, "read path %s fail", bucketPath)
	}

	list := ListBucketResult{}
	err = xml.Unmarshal(resp.Body(), &list)
	if err != nil && err.Error() != EOFError {
		return nil, err
	}
	var actualFiles []File
	for _, f := range list.Files {
		if f.Size > 0 {
			actualFiles = append(actualFiles, f)
		}
	}
	list.Files = actualFiles
	list.Count = len(actualFiles)

	// This is a dir
	if err == nil {
		addons := o.convertOSSFiles2Addons(list.Files, bucketPath)
		return addons, nil
	}

	return nil, errors.Wrap(err, "fail to read from OSS")
}

// convert2OSSItem convert OSS list result to Item
func (o ossReader) convertOSSFiles2Addons(files []File, bucketPath string) map[string]SourceMeta {
	const slash = "/"
	// calculate relativePath to path relative to bucket
	var relativePath = bucketPath
	if o.path != "" {
		relativePath = strings.TrimPrefix(relativePath, o.path)
		relativePath = strings.TrimPrefix(relativePath, "/")
	}
	var addonmetas = make(map[string]SourceMeta)
	var pathBuckets = make(map[string][]Item)
	for _, f := range files {
		fPath := strings.Split(path.Clean(f.Name), slash)
		if len(fPath) < 2 {
			continue
		}
		var addonName = fPath[0]
		if len(fPath) == 2 && fPath[1] == MetadataFileName {
			// This is the real addon
			addonmetas[addonName] = SourceMeta{Name: addonName}
		}
		pathList := pathBuckets[addonName]
		pathList = append(pathList, &OSSItem{
			path: path.Join(bucketPath, f.Name),
			tp:   f.Type,
			name: f.Name,
		})
		pathBuckets[addonName] = pathList
	}
	var addonList = make(map[string]SourceMeta)
	for k, v := range addonmetas {
		v.Items = pathBuckets[k]
		addonList[k] = v
	}

	return addonList
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
	return reader.ListAddonMeta(".")
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
