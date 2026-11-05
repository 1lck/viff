package i18n

// Message keys of the terminal tool window.
const (
	TermTitle        = Key("term.title")
	TermStripe       = Key("term.stripe")
	TermRestart      = Key("term.restart")
	TermClear        = Key("term.clear")
	TermHide         = Key("term.hide")
	TermStarting     = Key("term.starting")
	TermFailed       = Key("term.failed")
	TermRetry        = Key("term.retry")
	TermExited       = Key("term.exited")
	TermExitedCode   = Key("term.exitedCode")
	TermCloseRunning = Key("term.closeRunning")
	TermA11y         = Key("term.a11y")
	TermA11yScroll   = Key("term.a11y.scroll")
)

func init() {
	register(EN, map[Key]string{
		TermTitle:        "Terminal",
		TermStripe:       "Terminal\t⌥F12",
		TermRestart:      "Restart Shell",
		TermClear:        "Clear",
		TermHide:         "Hide Terminal",
		TermStarting:     "Starting shell…",
		TermFailed:       "Could not start the shell",
		TermRetry:        "Retry",
		TermExited:       "Process finished",
		TermExitedCode:   "Process finished with exit code %d",
		TermCloseRunning: "The terminal is still running. Closing the project ends its shell.",
		TermA11y:         "Terminal",
		TermA11yScroll:   "Terminal Scroll Bar",
	})
	register(ZHCN, map[Key]string{
		TermTitle:        "终端",
		TermStripe:       "终端\t⌥F12",
		TermRestart:      "重启 Shell",
		TermClear:        "清屏",
		TermHide:         "隐藏终端",
		TermStarting:     "正在启动 Shell…",
		TermFailed:       "无法启动 Shell",
		TermRetry:        "重试",
		TermExited:       "进程已结束",
		TermExitedCode:   "进程已结束，退出码 %d",
		TermCloseRunning: "终端仍在运行。关闭项目会结束它的 shell。",
		TermA11y:         "终端",
		TermA11yScroll:   "终端滚动条",
	})
}
