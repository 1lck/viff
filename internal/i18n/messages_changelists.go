package i18n

// Message keys of the Change Lists bar, its menu and its dialogs.
const (
	ChangelistTitle        = Key("changelist.title")
	ChangelistAllLists     = Key("changelist.allLists")
	ChangelistDefault      = Key("changelist.default")
	ChangelistCommitHint   = Key("changelist.commitHint")
	ChangelistNew          = Key("changelist.menu.new")
	ChangelistRename       = Key("changelist.menu.rename")
	ChangelistDelete       = Key("changelist.menu.delete")
	ChangelistCannotMove   = Key("changelist.error.move")
	ChangelistCannotDelete = Key("changelist.error.delete")
	ChangelistNameRequired = Key("changelist.error.nameRequired")
	ChangelistNameExists   = Key("changelist.error.nameExists")

	ChangelistNewTitle        = Key("changelist.dialog.newTitle")
	ChangelistCreate          = Key("changelist.dialog.create")
	ChangelistName            = Key("changelist.dialog.name")
	ChangelistNamePlaceholder = Key("changelist.dialog.namePlaceholder")
	ChangelistRenameTitle     = Key("changelist.dialog.renameTitle")
	ChangelistRenameButton    = Key("changelist.dialog.rename")
	ChangelistNewName         = Key("changelist.dialog.newName")
	ChangelistDeleteTitle     = Key("changelist.dialog.deleteTitle")
	ChangelistDeleteBody      = Key("changelist.dialog.deleteBody")
	ChangelistDeleteButton    = Key("changelist.dialog.delete")

	ChangelistFilesOne  = Key("changelist.files.one")
	ChangelistFilesMany = Key("changelist.files.many")
)

func init() {
	register(EN, map[Key]string{
		ChangelistTitle:        "Change Lists",
		ChangelistAllLists:     "All lists",
		ChangelistDefault:      "Default",
		ChangelistCommitHint:   "Commit includes all staged files",
		ChangelistNew:          "New Change List…",
		ChangelistRename:       "Rename Change List…",
		ChangelistDelete:       "Delete Change List…",
		ChangelistCannotMove:   "Cannot move to change list",
		ChangelistCannotDelete: "Cannot delete change list",
		ChangelistNameRequired: "Enter a name",
		ChangelistNameExists:   "A change list named %q already exists",

		ChangelistNewTitle:        "New Change List",
		ChangelistCreate:          "Create",
		ChangelistName:            "Name:",
		ChangelistNamePlaceholder: "e.g. Local changes",
		ChangelistRenameTitle:     "Rename Change List",
		ChangelistRenameButton:    "Rename",
		ChangelistNewName:         "New name:",
		ChangelistDeleteTitle:     "Delete Change List",
		ChangelistDeleteBody:      "Delete %q? %s will move to Default.",
		ChangelistDeleteButton:    "Delete",

		ChangelistFilesOne:  "%d file",
		ChangelistFilesMany: "%d files",
	})
	register(ZHCN, map[Key]string{
		ChangelistTitle:        "变更列表",
		ChangelistAllLists:     "全部列表",
		ChangelistDefault:      "默认",
		ChangelistCommitHint:   "提交会包含所有已暂存的文件",
		ChangelistNew:          "新建变更列表…",
		ChangelistRename:       "重命名变更列表…",
		ChangelistDelete:       "删除变更列表…",
		ChangelistCannotMove:   "无法移入变更列表",
		ChangelistCannotDelete: "无法删除变更列表",
		ChangelistNameRequired: "请输入名称",
		ChangelistNameExists:   "名为 %q 的变更列表已存在",

		ChangelistNewTitle:        "新建变更列表",
		ChangelistCreate:          "创建",
		ChangelistName:            "名称：",
		ChangelistNamePlaceholder: "例如：本地更改",
		ChangelistRenameTitle:     "重命名变更列表",
		ChangelistRenameButton:    "重命名",
		ChangelistNewName:         "新名称：",
		ChangelistDeleteTitle:     "删除变更列表",
		ChangelistDeleteBody:      "删除 %q？%s 会移动到默认列表。",
		ChangelistDeleteButton:    "删除",

		ChangelistFilesOne:  "%d 个文件",
		ChangelistFilesMany: "%d 个文件",
	})
}
