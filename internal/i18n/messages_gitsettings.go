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
	})
}
