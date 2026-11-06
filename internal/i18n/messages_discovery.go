package i18n

// Message keys of the repository discovery dialog. It is opened when the user
// chooses a folder that is not itself a Git repository, or from the
// Repositories overview.
const (
	DiscoverTitle         = Key("discover.title")
	DiscoverFind          = Key("discover.find")
	DiscoverAddTitle      = Key("discover.addTitle")
	DiscoverScanning      = Key("discover.scanning")
	DiscoverScanningShort = Key("discover.scanningShort")
	DiscoverFoundOne      = Key("discover.foundOne")
	DiscoverFoundMany     = Key("discover.foundMany")
	DiscoverNone          = Key("discover.none")
	DiscoverEmptyList     = Key("discover.emptyList")
	DiscoverFailed        = Key("discover.failed")
	DiscoverImport        = Key("discover.import")
	DiscoverSelectAll     = Key("discover.selectAll")
	DiscoverSelectNone    = Key("discover.selectNone")
	DiscoverList          = Key("discover.list")
	DiscoverTruncated     = Key("discover.truncated")
	DiscoverInaccessible  = Key("discover.inaccessible")
	DiscoverCannotAdd     = Key("discover.cannotAdd")
)

func init() {
	register(EN, map[Key]string{
		DiscoverTitle:         "Discover Repositories",
		DiscoverFind:          "Find Repositories…",
		DiscoverAddTitle:      "Find Repositories",
		DiscoverScanning:      "Scanning %s…",
		DiscoverScanningShort: "Scanning…",
		DiscoverFoundOne:      "Found 1 repository in %s",
		DiscoverFoundMany:     "Found %d repositories in %s",
		DiscoverNone:          "No Git repositories found in %s",
		DiscoverEmptyList:     "No repositories under this folder",
		DiscoverFailed:        "Cannot scan %s: %s",
		DiscoverImport:        "Import Selected",
		DiscoverSelectAll:     "Select All",
		DiscoverSelectNone:    "Select None",
		DiscoverList:          "Repositories to import",
		DiscoverTruncated:     "The scan stopped at its limit. Choose a narrower folder to find the rest.",
		DiscoverInaccessible:  "%d folders could not be read and were skipped.",
		DiscoverCannotAdd:     "Some repositories could not be added: %s",
	})
	register(ZHCN, map[Key]string{
		DiscoverTitle:         "发现仓库",
		DiscoverFind:          "查找仓库…",
		DiscoverAddTitle:      "查找仓库",
		DiscoverScanning:      "正在扫描 %s…",
		DiscoverScanningShort: "正在扫描…",
		DiscoverFoundOne:      "在 %s 中找到 1 个仓库",
		DiscoverFoundMany:     "找到 %d 个仓库：%s",
		DiscoverNone:          "在 %s 中未找到 Git 仓库",
		DiscoverEmptyList:     "此目录下没有仓库",
		DiscoverFailed:        "无法扫描 %s：%s",
		DiscoverImport:        "导入所选仓库",
		DiscoverSelectAll:     "全选",
		DiscoverSelectNone:    "全不选",
		DiscoverList:          "要导入的仓库",
		DiscoverTruncated:     "扫描已达到上限。请选择范围更小的目录查找其余仓库。",
		DiscoverInaccessible:  "%d 个目录无法读取，已跳过。",
		DiscoverCannotAdd:     "部分仓库无法添加：%s",
	})
}
