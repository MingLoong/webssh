<template>
  <div class="login-page">
    <div class="login-card">
      <h1 class="title">WebSSH</h1>
      <el-form :model="sshInfo" label-position="top">
        <el-row :gutter="12">
          <el-col :xs="24" :sm="12">
            <el-form-item label="主机地址">
              <el-input ref="hostnameInput" v-model="sshInfo.hostname" placeholder="服务器地址" />
            </el-form-item>
          </el-col>
          <el-col :xs="24" :sm="12">
            <el-form-item label="端口">
              <el-input v-model.number="sshInfo.port" placeholder="默认 22" />
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="12">
          <el-col :xs="24" :sm="12">
            <el-form-item label="用户名">
              <el-input ref="usernameInput" v-model="sshInfo.username" placeholder="用户名" />
            </el-form-item>
          </el-col>
          <el-col :xs="24" :sm="12">
            <el-form-item label="密码">
              <el-input
                ref="passwordInput"
                v-model="sshInfo.password"
                type="password"
                show-password
                placeholder="密码（或使用私钥）"
              />
            </el-form-item>
          </el-col>
        </el-row>

        <el-row :gutter="12">
          <el-col :xs="24" :sm="12">
            <el-form-item label="私钥">
              <el-upload
                class="upload-key"
                :show-file-list="false"
                :before-upload="handlePrivateKeyUpload"
                accept=".pem,.ppk,.key,.rsa,.id_rsa,.id_dsa,.txt"
              >
                <el-button size="small" icon="el-icon-folder-opened">选择密钥文件</el-button>
                <span class="file-name">{{ privateKeyFileName || '未选择文件' }}</span>
              </el-upload>
            </el-form-item>
          </el-col>
          <el-col :xs="24" :sm="12">
            <el-form-item label="私钥口令">
              <el-input v-model="sshInfo.passphrase" placeholder="可选" />
            </el-form-item>
          </el-col>
        </el-row>

        <el-form-item label="初始命令">
          <el-input v-model="sshInfo.command" placeholder="登录后执行，留空则不执行" />
        </el-form-item>

        <div class="actions">
          <el-button icon="el-icon-refresh" @click="onReset">重置</el-button>
          <el-button type="primary" icon="el-icon-link" @click="onGenerateLink">生成链接</el-button>
          <el-button type="success" @click="onConnect">连接 SSH</el-button>
        </div>

        <el-form-item v-if="generatedLink" class="generated-link">
          <el-input v-model="generatedLink" readonly>
            <template slot="append">
              <el-button icon="el-icon-document-copy" @click="copyLink" />
            </template>
          </el-input>
        </el-form-item>
      </el-form>
    </div>
  </div>
</template>

<script>
export default {
  data () {
    return {
      sshInfo: {
        hostname: '',
        port: '',
        username: '',
        password: '',
        privateKey: '',
        passphrase: '',
        command: ''
      },
      privateKeyFileName: '',
      generatedLink: ''
    }
  },
  watch: {
    sshInfo: {
      handler (newVal) {
        localStorage.setItem('connectionInfo', JSON.stringify(newVal))
      },
      deep: true
    }
  },
  created () {
    const savedInfo = localStorage.getItem('connectionInfo')
    if (!savedInfo) return

    const info = JSON.parse(savedInfo)
    this.sshInfo = {
      hostname: info.hostname || '',
      port: info.port || '',
      username: info.username || '',
      password: info.password || '',
      privateKey: info.privateKey || '',
      passphrase: info.passphrase || '',
      command: info.command || ''
    }

    if (info.privateKey) {
      this.privateKeyFileName = '已保存的密钥文件'
    }
  },
  methods: {
    onConnect () {
      sessionStorage.removeItem('sshInfo')

      if (!this.sshInfo.hostname) {
        this.$message.error('请输入主机地址')
        this.$nextTick(() => this.$refs.hostnameInput && this.$refs.hostnameInput.focus())
        return
      }
      if (!this.sshInfo.username) {
        this.$message.error('请输入用户名')
        this.$nextTick(() => this.$refs.usernameInput && this.$refs.usernameInput.focus())
        return
      }
      if (!this.sshInfo.password && !this.sshInfo.privateKey) {
        this.$message.error('请输入密码或上传私钥')
        this.$nextTick(() => this.$refs.passwordInput && this.$refs.passwordInput.focus())
        return
      }

      if (this.sshInfo.privateKey && this.sshInfo.privateKey.trim()) {
        this.sshInfo.password = ''
      } else if (this.sshInfo.password) {
        this.sshInfo.privateKey = ''
        this.sshInfo.passphrase = ''
        this.privateKeyFileName = ''
      }

      localStorage.setItem('connectionInfo', JSON.stringify({
        hostname: this.sshInfo.hostname,
        port: this.sshInfo.port || 22,
        username: this.sshInfo.username,
        password: this.sshInfo.password || '',
        privateKey: this.sshInfo.privateKey || '',
        passphrase: this.sshInfo.passphrase || '',
        command: this.sshInfo.command || ''
      }))

      const query = {
        hostname: encodeURIComponent(this.sshInfo.hostname),
        port: Number(this.sshInfo.port) || 22,
        username: encodeURIComponent(this.sshInfo.username),
        command: encodeURIComponent(this.sshInfo.command || '')
      }

      if (this.sshInfo.privateKey && this.sshInfo.privateKey.trim()) {
        sessionStorage.setItem('sshInfo', JSON.stringify(this.sshInfo))
        query.useKey = 1
      } else if (this.sshInfo.password) {
        query.password = btoa(this.sshInfo.password)
      }

      const url = this.$router.resolve({ path: '/terminal', query }).href
      window.open(url, '_blank')
    },
    onReset () {
      this.sshInfo = {
        hostname: '',
        port: '',
        username: '',
        password: '',
        command: '',
        privateKey: '',
        passphrase: ''
      }
      this.privateKeyFileName = ''
      this.generatedLink = ''
      localStorage.removeItem('connectionInfo')
      sessionStorage.removeItem('sshInfo')

      const fileInput = document.querySelector('.upload-key input[type="file"]')
      if (fileInput) fileInput.value = ''
    },
    onGenerateLink () {
      if (this.sshInfo.privateKey) {
        this.$message.warning('私钥登录不支持快捷链接，请改用密码登录')
        return
      }
      if (!this.sshInfo.hostname) {
        this.$message.error('请输入主机地址')
        this.$nextTick(() => this.$refs.hostnameInput && this.$refs.hostnameInput.focus())
        return
      }
      if (!this.sshInfo.username) {
        this.$message.error('请输入用户名')
        this.$nextTick(() => this.$refs.usernameInput && this.$refs.usernameInput.focus())
        return
      }
      if (!this.sshInfo.password) {
        this.$message.error('请输入密码后再生成链接')
        this.$nextTick(() => this.$refs.passwordInput && this.$refs.passwordInput.focus())
        return
      }

      const url = new URL(window.location.href)
      url.pathname = '/terminal'
      const cleanSshInfo = {}
      const infoToProcess = { ...this.sshInfo, port: this.sshInfo.port || 22 }
      for (const key in infoToProcess) {
        if (infoToProcess[key] === '' || infoToProcess[key] === null) continue
        cleanSshInfo[key] = key === 'password' ? btoa(infoToProcess[key]) : infoToProcess[key]
      }
      url.search = new URLSearchParams(cleanSshInfo).toString()
      this.generatedLink = url.href
    },
    copyLink () {
      if (!this.generatedLink) return
      navigator.clipboard.writeText(this.generatedLink).then(() => {
        this.$message.success('链接已复制')
      }).catch(err => {
        this.$message.error(`复制失败: ${err}`)
      })
    },
    handlePrivateKeyUpload (file) {
      this.sshInfo.password = ''
      const reader = new FileReader()
      reader.onload = (e) => {
        this.sshInfo.privateKey = e.target.result
        this.privateKeyFileName = file.name
      }
      reader.readAsText(file)
      return false
    }
  }
}
</script>

<style lang="scss" scoped>
.login-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: #f5f6f8;
  padding: 24px;
}

.login-card {
  width: 100%;
  max-width: 820px;
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  padding: 20px;
}

.title {
  margin: 0 0 12px;
  font-size: 24px;
  font-weight: 600;
  color: #111827;
  text-align: center;
}

.upload-key {
  display: flex;
  align-items: center;
}

.file-name {
  margin-left: 8px;
  color: #6b7280;
  font-size: 13px;
}

.actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.generated-link {
  margin-top: 12px;
}

@media (max-width: 768px) {
  .login-page {
    align-items: flex-start;
    padding: 12px;
  }

  .login-card {
    padding: 14px;
  }
}
</style>
