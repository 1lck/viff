package i18n

// Message keys of the notifications panel.
const (
	NoticesTitle    = Key("notices.title")
	NoticesEmpty    = Key("notices.empty")
	NoticesHide     = Key("notices.hide")
	NoticesClearAll = Key("notices.clearAll")
)

func init() {
	register(EN, map[Key]string{
		NoticesTitle:    "Notifications",
		NoticesEmpty:    "No notifications",
		NoticesHide:     "Hide",
		NoticesClearAll: "Clear All",
	})
	register(ZHCN, map[Key]string{
		NoticesTitle:    "通知",
		NoticesEmpty:    "暂无通知",
		NoticesHide:     "隐藏",
		NoticesClearAll: "全部清除",
	})
}
