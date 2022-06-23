/*
Copyright 2021 The KubeVela Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package resourcetracker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"

	"github.com/fatih/color"
	"github.com/gosuri/uitable"
	"github.com/gosuri/uitable/util/strutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	apicommon "github.com/oam-dev/kubevela/apis/core.oam.dev/common"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1alpha1"
	"github.com/oam-dev/kubevela/apis/core.oam.dev/v1beta1"
	"github.com/oam-dev/kubevela/pkg/multicluster"
	"github.com/oam-dev/kubevela/pkg/oam"
	"github.com/oam-dev/kubevela/pkg/utils"
	"github.com/oam-dev/kubevela/pkg/utils/common"
)

// ResourceDetailRetriever retriever to get details for resource
type ResourceDetailRetriever func(*resourceRow, string) error

// ResourceTreePrintOptions print options for resource tree
type ResourceTreePrintOptions struct {
	DetailRetriever ResourceDetailRetriever
	multicluster.ClusterNameMapper
	// MaxWidth if set, the detail part will auto wrap
	MaxWidth *int
	// Format for details
	Format string
}

const (
	resourceRowStatusUpdated     = "updated"
	resourceRowStatusNotDeployed = "not-deployed"
	resourceRowStatusOutdated    = "outdated"
)

type resourceRow struct {
	mr                   *v1beta1.ManagedResource
	status               string
	cluster              string
	namespace            string
	resourceName         string
	connectClusterUp     bool
	connectClusterDown   bool
	connectNamespaceUp   bool
	connectNamespaceDown bool
	applyTime            string
	details              string
}

func (options *ResourceTreePrintOptions) loadResourceRows(currentRT *v1beta1.ResourceTracker, historyRT []*v1beta1.ResourceTracker) []*resourceRow {
	var rows []*resourceRow
	if currentRT != nil {
		for _, mr := range currentRT.Spec.ManagedResources {
			if mr.Deleted {
				continue
			}
			rows = append(rows, buildResourceRow(mr, resourceRowStatusUpdated))
		}
	}
	for _, rt := range historyRT {
		for _, mr := range rt.Spec.ManagedResources {
			var matchedRow *resourceRow
			for _, row := range rows {
				if row.mr.ResourceKey() == mr.ResourceKey() {
					matchedRow = row
				}
			}
			if matchedRow == nil {
				rows = append(rows, buildResourceRow(mr, resourceRowStatusOutdated))
			}
		}
	}
	return rows
}

func (options *ResourceTreePrintOptions) sortRows(rows []*resourceRow) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].mr.Cluster != rows[j].mr.Cluster {
			return rows[i].mr.Cluster < rows[j].mr.Cluster
		}
		if rows[i].mr.Namespace != rows[j].mr.Namespace {
			return rows[i].mr.Namespace < rows[j].mr.Namespace
		}
		return rows[i].mr.ResourceKey() < rows[j].mr.ResourceKey()
	})
}

func (options *ResourceTreePrintOptions) fillResourceRows(rows []*resourceRow, colsWidth []int) {
	for i := 0; i < 4; i++ {
		colsWidth[i] = 10
	}
	connectLastRow := func(rowIdx int, cluster bool, namespace bool) {
		rows[rowIdx].connectClusterUp = cluster
		rows[rowIdx-1].connectClusterDown = cluster
		rows[rowIdx].connectNamespaceUp = namespace
		rows[rowIdx-1].connectNamespaceDown = namespace
	}
	for rowIdx, row := range rows {
		if row.mr.Cluster == "" {
			row.mr.Cluster = multicluster.ClusterLocalName
		}
		if row.mr.Namespace == "" {
			row.mr.Namespace = "-"
		}
		row.cluster, row.namespace, row.resourceName = options.ClusterNameMapper.GetClusterName(row.mr.Cluster), row.mr.Namespace, fmt.Sprintf("%s/%s", row.mr.Kind, row.mr.Name)
		if row.status == resourceRowStatusNotDeployed {
			row.resourceName = "-"
		}
		if rowIdx > 0 && row.mr.Cluster == rows[rowIdx-1].mr.Cluster {
			connectLastRow(rowIdx, true, false)
			row.cluster = ""
			if row.mr.Namespace == rows[rowIdx-1].mr.Namespace {
				connectLastRow(rowIdx, true, true)
				row.namespace = ""
			}
		}
		for i, val := range []string{row.cluster, row.namespace, row.resourceName, row.status} {
			if size := len(val) + 1; size > colsWidth[i] {
				colsWidth[i] = size
			}
		}
	}
	for rowIdx := len(rows); rowIdx >= 1; rowIdx-- {
		if rowIdx == len(rows) || rows[rowIdx].cluster != "" {
			for j := rowIdx - 1; j >= 1; j-- {
				if rows[j].cluster == "" && rows[j].namespace == "" {
					connectLastRow(j, false, rows[j].connectNamespaceUp)
					if j+1 < len(rows) {
						connectLastRow(j+1, false, rows[j+1].connectNamespaceUp)
					}
					continue
				}
				break
			}
		}
	}

	// add extra spaces for tree connectors
	colsWidth[0] += 4
	colsWidth[1] += 4
}

const (
	applyTimeWidth = 20
	detailMinWidth = 20
)

func (options *ResourceTreePrintOptions) _getWidthForDetails(colsWidth []int) int {
	detailWidth := 0
	if options.MaxWidth == nil {
		return math.MaxInt
	}
	detailWidth = *options.MaxWidth - applyTimeWidth
	for _, width := range colsWidth {
		detailWidth -= width
	}
	// if the space for details exceeds the max allowed width, give up wrapping lines
	if detailWidth < detailMinWidth {
		detailWidth = math.MaxInt
	}
	return detailWidth
}

func (options *ResourceTreePrintOptions) _wrapDetails(detail string, width int) (lines []string) {
	for _, row := range strings.Split(detail, "\n") {
		var sb strings.Builder
		row = strings.ReplaceAll(row, "\t", " ")
		sep := "  "
		if options.Format == "raw" {
			sep = "\n"
		}
		for _, token := range strings.Split(row, sep) {
			if sb.Len()+len(token)+2 <= width {
				if sb.Len() > 0 {
					sb.WriteString(sep)
				}
				sb.WriteString(token)
			} else {
				if sb.Len() > 0 {
					lines = append(lines, sb.String())
					sb.Reset()
				}
				offset := 0
				for {
					if offset+width > len(token) {
						break
					}
					lines = append(lines, token[offset:offset+width])
					offset += width
				}
				sb.WriteString(token[offset:])
			}
		}
		if sb.Len() > 0 {
			lines = append(lines, sb.String())
		}
	}
	if len(lines) == 0 {
		lines = []string{""}
	}
	return lines
}

func (options *ResourceTreePrintOptions) writeResourceTree(writer io.Writer, rows []*resourceRow, colsWidth []int) {
	writePaddedString := func(sb *strings.Builder, head string, tail string, width int) {
		sb.WriteString(head)
		for c := strutil.StringWidth(head) + strutil.StringWidth(tail); c < width; c++ {
			sb.WriteByte(' ')
		}
		sb.WriteString(tail)
	}

	var headerWriter strings.Builder
	for colIdx, colName := range []string{"CLUSTER", "NAMESPACE", "RESOURCE", "STATUS"} {
		writePaddedString(&headerWriter, colName, "", colsWidth[colIdx])
	}
	if options.DetailRetriever != nil {
		writePaddedString(&headerWriter, "APPLY_TIME", "", applyTimeWidth)
		_, _ = writer.Write([]byte(headerWriter.String() + "DETAIL" + "\n"))
	} else {
		_, _ = writer.Write([]byte(headerWriter.String() + "\n"))
	}

	connectorColorizer := color.WhiteString
	outdatedColorizer := color.WhiteString
	detailWidth := options._getWidthForDetails(colsWidth)

	for _, row := range rows {
		if options.DetailRetriever != nil && row.status != resourceRowStatusNotDeployed {
			if err := options.DetailRetriever(row, options.Format); err != nil {
				row.details = "Error: " + err.Error()
			}
		}
		for lineIdx, line := range options._wrapDetails(row.details, detailWidth) {
			var sb strings.Builder
			rscName, rscStatus, applyTime := row.resourceName, row.status, row.applyTime
			if row.status != resourceRowStatusUpdated {
				rscName, rscStatus, applyTime, line = outdatedColorizer(row.resourceName), outdatedColorizer(row.status), outdatedColorizer(applyTime), outdatedColorizer(line)
			}
			if lineIdx == 0 {
				writePaddedString(&sb, row.cluster, connectorColorizer(utils.GetBoxDrawingString(row.connectClusterUp, row.connectClusterDown, row.cluster != "", row.namespace != "", 1, 1))+" ", colsWidth[0])
				writePaddedString(&sb, row.namespace, connectorColorizer(utils.GetBoxDrawingString(row.connectNamespaceUp, row.connectNamespaceDown, row.namespace != "", true, 1, 1))+" ", colsWidth[1])
				writePaddedString(&sb, rscName, "", colsWidth[2])
				writePaddedString(&sb, rscStatus, "", colsWidth[3])
			} else {
				writePaddedString(&sb, "", connectorColorizer(utils.GetBoxDrawingString(row.connectClusterDown, row.connectClusterDown, false, false, 1, 1))+" ", colsWidth[0])
				writePaddedString(&sb, "", connectorColorizer(utils.GetBoxDrawingString(row.connectNamespaceDown, row.connectNamespaceDown, false, false, 1, 1))+" ", colsWidth[1])
				writePaddedString(&sb, "", "", colsWidth[2])
				writePaddedString(&sb, "", "", colsWidth[3])
			}

			if options.DetailRetriever != nil {
				if lineIdx != 0 {
					applyTime = ""
				}
				writePaddedString(&sb, applyTime, "", applyTimeWidth)
			}
			_, _ = writer.Write([]byte(sb.String() + line + "\n"))
		}
	}
}

func (options *ResourceTreePrintOptions) addNonExistingPlacementToRows(placements []v1alpha1.PlacementDecision, rows []*resourceRow) []*resourceRow {
	existingClusters := map[string]struct{}{}
	for _, row := range rows {
		existingClusters[row.mr.Cluster] = struct{}{}
	}
	for _, p := range placements {
		if _, found := existingClusters[p.Cluster]; !found {
			rows = append(rows, &resourceRow{
				mr: &v1beta1.ManagedResource{
					ClusterObjectReference: apicommon.ClusterObjectReference{Cluster: p.Cluster},
				},
				status: resourceRowStatusNotDeployed,
			})
		}
	}
	return rows
}

// PrintResourceTree print resource tree to writer
func (options *ResourceTreePrintOptions) PrintResourceTree(writer io.Writer, currentPlacements []v1alpha1.PlacementDecision, currentRT *v1beta1.ResourceTracker, historyRT []*v1beta1.ResourceTracker) {
	rows := options.loadResourceRows(currentRT, historyRT)
	rows = options.addNonExistingPlacementToRows(currentPlacements, rows)
	options.sortRows(rows)

	colsWidth := make([]int, 4)
	options.fillResourceRows(rows, colsWidth)

	options.writeResourceTree(writer, rows, colsWidth)
}

type tableRoundTripper struct {
	rt http.RoundTripper
}

// RoundTrip mutate the request header to let apiserver return table data
func (rt tableRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Accept", strings.Join([]string{
		fmt.Sprintf("application/json;as=Table;v=%s;g=%s", metav1.SchemeGroupVersion.Version, metav1.GroupName),
		"application/json",
	}, ","))
	return rt.rt.RoundTrip(req)
}

// RetrieveKubeCtlGetMessageGenerator get details like kubectl get
func RetrieveKubeCtlGetMessageGenerator(cfg *rest.Config) (ResourceDetailRetriever, error) {
	cfg.Wrap(func(rt http.RoundTripper) http.RoundTripper {
		return tableRoundTripper{rt: rt}
	})
	cli, err := client.New(cfg, client.Options{Scheme: common.Scheme})
	if err != nil {
		return nil, err
	}
	return func(row *resourceRow, format string) error {
		mr := row.mr
		un := &unstructured.Unstructured{}
		un.SetAPIVersion(mr.APIVersion)
		un.SetKind(mr.Kind)
		if err = cli.Get(multicluster.ContextWithClusterName(context.Background(), mr.Cluster), mr.NamespacedName(), un); err != nil {
			return err
		}
		un.SetAPIVersion(metav1.SchemeGroupVersion.String())
		un.SetKind("Table")
		table := &metav1.Table{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(un.Object, table); err != nil {
			return err
		}

		obj := &unstructured.Unstructured{}
		if err := json.Unmarshal(table.Rows[0].Object.Raw, obj); err == nil {
			row.applyTime = oam.GetLastAppliedTime(obj).Format("2006-01-02 15:04:05")
		}

		switch format {
		case "raw":
			raw := table.Rows[0].Object.Raw
			if annotations := obj.GetAnnotations(); annotations != nil && annotations[oam.AnnotationLastAppliedConfig] != "" {
				raw = []byte(annotations[oam.AnnotationLastAppliedConfig])
			}
			bs, err := yaml.JSONToYAML(raw)
			if err != nil {
				return err
			}
			row.details = string(bs)
		case "table":
			tab := uitable.New()
			var tabHeaders, tabValues []interface{}
			for cid, column := range table.ColumnDefinitions {
				if column.Name == "Name" || column.Name == "Created At" || column.Priority != 0 {
					continue
				}
				tabHeaders = append(tabHeaders, column.Name)
				tabValues = append(tabValues, table.Rows[0].Cells[cid])
			}
			tab.AddRow(tabHeaders...)
			tab.AddRow(tabValues...)
			row.details = tab.String()
		default: // inline / wide / list
			var entries []string
			for cid, column := range table.ColumnDefinitions {
				if column.Name == "Name" || column.Name == "Created At" || (format == "inline" && column.Priority != 0) {
					continue
				}
				entries = append(entries, fmt.Sprintf("%s: %v", column.Name, table.Rows[0].Cells[cid]))
			}
			if format == "inline" || format == "wide" {
				row.details = strings.Join(entries, "  ")
			} else {
				row.details = strings.Join(entries, "\n")
			}
		}
		return nil
	}, nil
}

func buildResourceRow(mr v1beta1.ManagedResource, resourceStatus string) *resourceRow {
	rr := &resourceRow{
		mr:     mr.DeepCopy(),
		status: resourceStatus,
	}
	if rr.mr.Cluster == "" {
		rr.mr.Cluster = multicluster.ClusterLocalName
	}
	return rr
}
