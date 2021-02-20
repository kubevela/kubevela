package cli

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/fatih/color"
	"github.com/gosuri/uitable"
	"github.com/mitchellh/hashstructure/v2"

	"github.com/oam-dev/kubevela/apis/types"
	"github.com/oam-dev/kubevela/pkg/utils/system"
	cmdutil "github.com/oam-dev/kubevela/pkg/utils/util"
	"github.com/oam-dev/kubevela/references/plugins"
)

type refreshStatus string

const (
	added     refreshStatus = "Added"
	updated   refreshStatus = "Updated"
	unchanged refreshStatus = "Unchanged"
	deleted   refreshStatus = "Deleted"
)

const (
	refreshInterval = 5 * time.Minute
)

// RefreshDefinitions will sync local capabilities with cluster installed ones
func RefreshDefinitions(ctx context.Context, c types.Args, ioStreams cmdutil.IOStreams, silentOutput, enforceRefresh bool) error {
	dir, _ := system.GetCapabilityDir()
	oldCaps, err := plugins.LoadAllInstalledCapability()
	if err != nil {
		return err
	}
	useCached, err := useCacheInsteadRefresh(dir, refreshInterval)
	if err != nil {
		return err
	}
	if !enforceRefresh && useCached {
		// use local capabilities instead of fetching from cluster
		printRefreshReport(nil, oldCaps, ioStreams, silentOutput, true)
		return nil
	}

	syncedTemplates, warnings, err := plugins.SyncDefinitionsToLocal(ctx, c, dir)
	if err != nil {
		return err
	}
	for _, w := range warnings {
		ioStreams.Infof(w)
	}
	plugins.RemoveLegacyTemps(syncedTemplates, dir)
	printRefreshReport(syncedTemplates, oldCaps, ioStreams, silentOutput, false)
	return nil
}

// silent indicates whether output existing caps if no change occurs. If false, output all existing caps.
func printRefreshReport(newCaps, oldCaps []types.Capability, io cmdutil.IOStreams, silent, useCached bool) {
	var report map[refreshStatus][]types.Capability
	if useCached {
		report = map[refreshStatus][]types.Capability{
			added:     make([]types.Capability, 0),
			updated:   make([]types.Capability, 0),
			unchanged: oldCaps,
			deleted:   make([]types.Capability, 0),
		}
	} else {
		report = refreshResultReport(newCaps, oldCaps)
	}
	table := newUITable()
	table.MaxColWidth = 80
	table.AddRow("TYPE", "CATEGORY", "DESCRIPTION")

	if len(report[added]) == 0 && len(report[updated]) == 0 && len(report[deleted]) == 0 {
		// no change occurs, just show all existing caps
		// always show workload at first
		for _, cap := range report[unchanged] {
			if cap.Type == types.TypeWorkload {
				table.AddRow(cap.Name, cap.Type, cap.Description)
			}
		}
		for _, cap := range report[unchanged] {
			if cap.Type == types.TypeTrait {
				table.AddRow(cap.Name, cap.Type, cap.Description)
			}
		}
		if !silent {
			io.Infof("Automatically discover capabilities successfully %s(no changes)\n\n", emojiSucceed)
			io.Info(table.String())
		}
		return
	}

	io.Infof("Automatically discover capabilities successfully %sAdd(%s) Update(%s) Delete(%s)\n\n",
		emojiSucceed,
		green.Sprint(len(report[added])),
		yellow.Sprint(len(report[updated])),
		red.Sprint(len(report[deleted])))
	// show added/updated/deleted cpas
	addStsRow(added, report, table)
	addStsRow(updated, report, table)
	addStsRow(deleted, report, table)
	io.Info(table.String())
	io.Info()
}

func addStsRow(sts refreshStatus, report map[refreshStatus][]types.Capability, t *uitable.Table) {
	caps := report[sts]
	if len(caps) == 0 {
		return
	}
	var stsIcon string
	var stsColor *color.Color
	switch sts {
	case added:
		stsIcon = "+"
		stsColor = green
	case updated:
		stsIcon = "*"
		stsColor = yellow
	case deleted:
		stsIcon = "-"
		stsColor = red
	case unchanged:
		// normal color display
	}
	for _, cap := range caps {
		t.AddRow(
			// color.New(color.Bold).Sprint(stsColor.Sprint(stsIcon)),
			stsColor.Sprintf("%s%s", stsIcon, cap.Name),
			stsColor.Sprint(cap.Type),
			stsColor.Sprint(cap.Description))
	}
}

func refreshResultReport(newCaps, oldCaps []types.Capability) map[refreshStatus][]types.Capability {
	dir, _ := system.GetCapabilityDir()
	cachedHash := readCapDefHashFromLocal(dir)
	newHash := map[string]string{}
	for _, newCap := range newCaps {
		h, err := hashstructure.Hash(newCap, hashstructure.FormatV2, nil)
		if err != nil {
			continue
		}
		newHash[newCap.Name] = strconv.FormatUint(h, 10)
	}

	report := map[refreshStatus][]types.Capability{
		added:     make([]types.Capability, 0),
		updated:   make([]types.Capability, 0),
		unchanged: make([]types.Capability, 0),
		deleted:   make([]types.Capability, 0),
	}
	for _, newCap := range newCaps {
		found := false
		for _, oldCap := range oldCaps {
			if newCap.Name == oldCap.Name {
				found = true
				break
			}
		}
		if !found {
			report[added] = append(report[added], newCap)
		}
	}
	for _, oldCap := range oldCaps {
		found := false
		for _, newCap := range newCaps {
			if oldCap.Name == newCap.Name {
				found = true
				// use cached hash to determine whether the cap is changed
				if h, ok := cachedHash[newCap.Name]; ok {
					if h == newHash[newCap.Name] {
						report[unchanged] = append(report[unchanged], newCap)
					} else {
						report[updated] = append(report[updated], newCap)
					}
					break
				}
				// in case of missing cache, use Equal func to compare
				if types.EqualCapability(oldCap, newCap) {
					report[unchanged] = append(report[unchanged], newCap)
				} else {
					report[updated] = append(report[updated], newCap)
				}
				break
			}
		}
		if !found {
			report[deleted] = append(report[deleted], oldCap)
		}
	}
	_ = writeCapDefHashIntoLocal(dir, newHash)
	return report
}

// useCacheInsteadRefresh checks whether use cached capabilities instead of refresh from cluster
// a timestamp records the time when refresh from cluster last time
// if duration since last time refresh DOES NOT exceed `cacheExpiredDuration`
// use cached capabilities instead of refresh from cluster
// else refresh from cluster and refresh the timestamp
func useCacheInsteadRefresh(capDir string, cacheExpiredDuration time.Duration) (bool, error) {
	currentTimestamp := strconv.FormatInt(time.Now().Unix(), 10)
	tmpDir := filepath.Join(capDir, ".tmp")
	timeFilePath := filepath.Join(tmpDir, ".lasttimerefresh")
	exist, _ := system.CreateIfNotExist(tmpDir)
	if !exist {
		// file saving timestamp is not created yet, create and refresh the timestamp
		if err := ioutil.WriteFile(timeFilePath, []byte(currentTimestamp), 0600); err != nil {
			return false, err
		}
		return false, nil
	}
	r, err := ioutil.ReadFile(filepath.Clean(timeFilePath))
	if err != nil {
		if os.IsNotExist(err) {
			// tmpDir exists but `.lasttimerefresh` file doesn't
			if err := ioutil.WriteFile(timeFilePath, []byte(currentTimestamp), 0600); err != nil {
				return false, err
			}
			return false, nil
		}
		return false, err
	}
	i, err := strconv.ParseInt(string(r), 10, 64)
	if err != nil {
		return false, err
	}
	lt := time.Unix(i, 0)
	if time.Since(lt) > cacheExpiredDuration {
		// cache is expired, refresh the timestamp
		if err := ioutil.WriteFile(timeFilePath, []byte(currentTimestamp), 0600); err != nil {
			return false, err
		}
		return false, nil
	}
	// cache is not expired
	return true, nil
}

// each capability has a hash value cached in local capability dir
// hash value is used to compare local capability with one from cluster
// refresh report will show all changed capabilities
func readCapDefHashFromLocal(capDir string) map[string]string {
	r := map[string]string{}
	tmpDir := filepath.Join(capDir, ".tmp")
	hashFilePath := filepath.Join(tmpDir, ".capabilityhash")
	if exist, err := system.CreateIfNotExist(tmpDir); !exist || err != nil {
		return r
	}
	if _, err := os.Stat(hashFilePath); os.IsNotExist(err) {
		return r
	}
	hashData, err := ioutil.ReadFile(filepath.Clean(hashFilePath))
	if err != nil {
		return r
	}
	if err := json.Unmarshal(hashData, &r); err != nil {
		return r
	}
	return r
}

func writeCapDefHashIntoLocal(capDir string, hashData map[string]string) error {
	tmpDir := filepath.Join(capDir, ".tmp")
	hashFilePath := filepath.Join(tmpDir, ".capabilityhash")
	if _, err := system.CreateIfNotExist(tmpDir); err != nil {
		return err
	}
	data, err := json.Marshal(hashData)
	if err != nil {
		return err
	}
	if err := ioutil.WriteFile(hashFilePath, data, 0600); err != nil {
		return err
	}
	return nil
}
