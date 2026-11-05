package i18n

// Message keys of the Git page of the settings dialog.
const (
	GitSSHCommand       = Key("gitsettings.ssh")
	GitSSHPlaceholder   = Key("gitsettings.sshPlaceholder")
	GitGPGProgram       = Key("gitsettings.gpg")
	GitGPGPlaceholder   = Key("gitsettings.gpgPlaceholder")
	GitSignFormat       = Key("gitsettings.signFormat")
	GitSignDefault      = Key("gitsettings.signDefault")
	GitSigningKey       = Key("gitsettings.signingKey")
	GitSigningKeyHint   = Key("gitsettings.signingKeyHint")
	GitSignByDefault    = Key("gitsettings.signByDefault")
	GitCredentialHelper = Key("gitsettings.credentialHelper")
	GitCredentialHint   = Key("gitsettings.credentialHint")
	GitCommandNotFound  = Key("gitsettings.commandNotFound")
	GitAIEndpoint       = Key("gitsettings.aiEndpoint")
	GitAIEndpointHint   = Key("gitsettings.aiEndpointHint")
	GitAIModel          = Key("gitsettings.aiModel")
	GitAIModelHint      = Key("gitsettings.aiModelHint")
	GitAIKey            = Key("gitsettings.aiKey")
	GitAIKeyHint        = Key("gitsettings.aiKeyHint")
	GitAIProtocol       = Key("gitsettings.aiProtocol")
	GitAIProtoChat      = Key("gitsettings.aiProtocolChat")
	GitAIProtoResponses = Key("gitsettings.aiProtocolResponses")
	GitAIProtoAnthropic = Key("gitsettings.aiProtocolAnthropic")
	GitAIHint           = Key("gitsettings.aiHint")
	GitAIProvider       = Key("gitsettings.aiProvider")
	GitAIProviderCustom = Key("gitsettings.aiProviderCustom")
	GitAIName           = Key("gitsettings.aiName")
	GitAIBaseURL        = Key("gitsettings.aiBaseURL")
	GitAIFetchModels    = Key("gitsettings.aiFetchModels")
	GitAIFetching       = Key("gitsettings.aiFetching")
)

func init() {
	register(EN, map[Key]string{
		GitSSHCommand:       "SSH command:",
		GitSSHPlaceholder:   "Empty: Git's own core.sshCommand",
		GitGPGProgram:       "GPG program:",
		GitGPGPlaceholder:   "Empty: gpg from PATH",
		GitSignFormat:       "Signing format:",
		GitSignDefault:      "Git default",
		GitSigningKey:       "Signing key:",
		GitSigningKeyHint:   "Empty: user.signingkey",
		GitSignByDefault:    "Sign commits by default",
		GitCredentialHelper: "Credential helper:",
		GitCredentialHint:   "Empty: Git's own credential.helper",
		GitCommandNotFound:  "Not found: %s",
		GitAIEndpoint:       "AI endpoint:",
		GitAIEndpointHint:   "Empty turns AI commit messages off; e.g. https://api.openai.com/v1",
		GitAIModel:          "AI model:",
		GitAIModelHint:      "e.g. gpt-4o-mini",
		GitAIKey:            "AI key:",
		GitAIKeyHint:        "Sent as a Bearer token, stored in plain text",
		GitAIProtocol:       "AI protocol:",
		GitAIProtoChat:      "Chat Completions (/chat/completions)",
		GitAIProtoResponses: "Responses (/responses)",
		GitAIProtoAnthropic: "Anthropic Messages (/v1/messages)",
		GitAIHint:           "Leave the endpoint empty to turn AI commit messages off; the key is stored in plain text.",
		GitAIProvider:       "Provider:",
		GitAIProviderCustom: "Custom…",
		GitAIName:           "Name:",
		GitAIBaseURL:        "Base URL:",
		GitAIFetchModels:    "Get models",
		GitAIFetching:       "Loading models…",
	})
	register(ZHCN, map[Key]string{
		GitSSHCommand:       "SSH 命令：",
		GitSSHPlaceholder:   "留空则用 Git 自带的 core.sshCommand",
		GitGPGProgram:       "GPG 程序：",
		GitGPGPlaceholder:   "留空则用 PATH 中的 gpg",
		GitSignFormat:       "签名格式：",
		GitSignDefault:      "Git 默认",
		GitSigningKey:       "签名密钥：",
		GitSigningKeyHint:   "留空则用 user.signingkey",
		GitSignByDefault:    "默认签名提交",
		GitCredentialHelper: "凭据助手：",
		GitCredentialHint:   "留空则用 Git 自带的 credential.helper",
		GitCommandNotFound:  "未找到：%s",
		GitAIEndpoint:       "AI 端点：",
		GitAIEndpointHint:   "留空则不启用 AI 生成；例如 https://api.openai.com/v1",
		GitAIModel:          "AI 模型：",
		GitAIModelHint:      "例如 gpt-4o-mini",
		GitAIKey:            "AI 密钥：",
		GitAIKeyHint:        "作为 Bearer 令牌发送，明文保存在设置里",
		GitAIProtocol:       "AI 协议：",
		GitAIProtoChat:      "Chat Completions（/chat/completions）",
		GitAIProtoResponses: "Responses（/responses）",
		GitAIProtoAnthropic: "Anthropic Messages（/v1/messages）",
		GitAIHint:           "端点留空即关闭 AI 生成；密钥以明文保存在设置文件里。",
		GitAIProvider:       "提供商：",
		GitAIProviderCustom: "自定义…",
		GitAIName:           "名称：",
		GitAIBaseURL:        "Base URL：",
		GitAIFetchModels:    "获取模型",
		GitAIFetching:       "正在获取模型…",
	})
}
