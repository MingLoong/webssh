<template>
  <div class="terminal-page-wrapper">
    <div class="terminal-page-container">
      <div class="terminal-area">
        <div id="xterm-container"></div>
      </div>
      <div class="sftp-resizer" @mousedown.prevent="startResizeSftp"></div>
      <div class="file-tree" :class="{ 'is-visible': isSftpVisible }" :style="fileTreeStyle">
        <FileList />
      </div>
    </div>
    <div class="terminal-footer">
      <button @click="toggleSftpPanel" class="sftp-toggle-btn" title="文件管理">
        <i class="fas fa-folder-open" style="margin-left: 15px;"></i>
      </button>
    </div>
  </div>
</template>

<script>
import { checkSSH } from '@/api/common'
import { Terminal } from 'xterm'
import { FitAddon } from 'xterm-addon-fit'
import { AttachAddon } from 'xterm-addon-attach'
import FileList from '@/components/FileList'

export default {
  name: 'Terminal',
  components: { FileList },
  computed: {
    fileTreeStyle () {
      if (this.windowWidth <= 768) {
        return {}
      }
      return { width: `${this.sftpWidth}px` }
    }
  },
  data () {
    return {
      term: null,
      ws: null,
      resetClose: false,
      ssh: null,
      savePass: false,
      fontSize: 15,
      isSftpVisible: false,
      sftpWidth: 350,
      minSftpWidth: 260,
      isResizingSftp: false,
      resizeStartX: 0,
      resizeStartWidth: 350,
      windowWidth: 1024
    }
  },
  mounted () {
    this.windowWidth = window.innerWidth
    window.addEventListener('resize', this.handleWindowResize)
    this.$nextTick(() => {
      this.createTerm()
    })
  },
  methods: {
    handleWindowResize () {
      this.windowWidth = window.innerWidth
    },
    startResizeSftp (e) {
      if (this.windowWidth <= 768) {
        return
      }
      this.isResizingSftp = true
      this.resizeStartX = e.clientX
      this.resizeStartWidth = this.sftpWidth
      document.addEventListener('mousemove', this.onResizeSftp)
      document.addEventListener('mouseup', this.stopResizeSftp)
      document.body.style.cursor = 'col-resize'
      document.body.style.userSelect = 'none'
    },
    onResizeSftp (e) {
      if (!this.isResizingSftp) {
        return
      }
      const container = document.querySelector('.terminal-page-container')
      const maxWidth = container ? Math.max(this.minSftpWidth, container.clientWidth - 320) : 720
      const delta = this.resizeStartX - e.clientX
      const nextWidth = this.resizeStartWidth + delta
      this.sftpWidth = Math.min(maxWidth, Math.max(this.minSftpWidth, nextWidth))
    },
    stopResizeSftp () {
      if (!this.isResizingSftp) {
        return
      }
      this.isResizingSftp = false
      document.removeEventListener('mousemove', this.onResizeSftp)
      document.removeEventListener('mouseup', this.stopResizeSftp)
      document.body.style.cursor = ''
      document.body.style.userSelect = ''
    },
    toggleSftpPanel () {
      this.isSftpVisible = !this.isSftpVisible
    },
    setSSH () {
      this.$store.commit('SET_SSH', this.ssh)
    },
    createTerm () {
      const sshInfo = this.$store.state.sshInfo
      if (!sshInfo || !sshInfo.hostname) {
        this.$message.error('无效的连接信息，正在返回登录页')
        this.$router.push('/')
        return
      }

      const termWeb = document.getElementById('xterm-container')
      if (!termWeb) {
        console.error('Terminal container #xterm-container not found.')
        return
      }

      const sshReq = this.$store.getters.sshReq
      this.close()
      const prefix = process.env.NODE_ENV === 'production' ? '' : '/api'
      const fitAddon = new FitAddon()

      this.term = new Terminal({
        cursorBlink: true,
        cursorStyle: 'bar',
        cursorWidth: 4,
        fontFamily: 'DejaVu Sans Mono, monospace',
        fontSize: this.fontSize,
        theme: {
          background: '#000000',
          foreground: '#ffffff',
          cursor: '#ffffff',
          selection: '#daffe77a',
          blue: '#1981ff',
          brightMagenta: '#e879f9',
          brightBlue: '#6eb0ff'
        }
      })

      this.term.loadAddon(fitAddon)
      this.term.open(termWeb)
      this.term.focus()
      this.term.write('\x1b[1;1H\x1b[1;32m正在连接，请稍后...\x1b[0m\r\n')

      try {
        fitAddon.fit()
      } catch (e) {
        // Ignore transient fit errors during initial render.
      }

      const self = this
      const heartCheck = {
        timeout: 5000,
        intervalObj: null,
        stop: function () {
          clearInterval(this.intervalObj)
        },
        start: function () {
          this.intervalObj = setInterval(function () {
            if (self.ws !== null && self.ws.readyState === 1) {
              self.ws.send('ping')
            }
          }, this.timeout)
        }
      }

      let closeTip = '连接超时已关闭'
      if (this.$store.state.language === 'en') {
        closeTip = 'Connection timed out!'
      }

      this.ws = new WebSocket(`${(location.protocol === 'http:' ? 'ws' : 'wss')}://${location.host}${prefix}/term?sshInfo=${sshReq}&rows=${this.term.rows}&cols=${this.term.cols}&closeTip=${closeTip}`)

      this.ws.onopen = () => {
        self.connected()
        heartCheck.start()
        self._initCmdSent = false
      }

      this.ws.onmessage = (event) => {
        if (typeof event.data === 'string') {
          setTimeout(() => {
            if (!self._initCmdSent && self.ssh) {
              const term = self.term
              if (!term || !term.buffer || !term.buffer.active) return

              const currentLineNumber = term.buffer.active.baseY + term.buffer.active.cursorY
              const line = term.buffer.active.getLine(currentLineNumber)
              if (line) {
                const lineText = line.translateToString()
                if (/[>$#%]\s*$/.test(lineText.trimEnd())) {
                  self._initCmdSent = true
                  self.term.write('\x1b[s\x1b[1;1H\x1b[2K\x1b[u')
                  if (self.ssh.command) {
                    setTimeout(() => {
                      if (self.ws && self.ws.readyState === 1) {
                        self.ws.send(self.ssh.command + '\r')
                      }
                    }, 100)
                  }
                }
              }
            }
          }, 10)
        }
      }

      this.ws.onclose = () => {
        if (!self.resetClose) {
          if (self.ssh && !this.savePass) {
            this.$store.commit('SET_PASS', '')
            self.ssh.password = ''
          }
          this.$message({
            message: this.$t('wsClose'),
            type: 'warning',
            duration: 0,
            showClose: true
          })
          this.ws = null
        }

        heartCheck.stop()
        self.resetClose = false
        if (self.ws !== null && self.ws.readyState === 1) {
          self.ws.send(`resize:${self.term.rows}:${self.term.cols}`)
        }
      }

      this.ws.onerror = () => {}

      const attachAddon = new AttachAddon(this.ws, { bidirectional: false })
      this.term.loadAddon(attachAddon)

      this.term.onData(data => {
        if (self.ws && self.ws.readyState === 1) {
          self.ws.send(data)
        }
      })

      this.term.attachCustomKeyEventHandler((e) => {
        const keyArray = ['F5', 'F11', 'F12']
        if (keyArray.indexOf(e.key) > -1) {
          return false
        }
        if (e.ctrlKey && e.key === 'v') {
          document.execCommand('copy')
          return false
        }
        if (e.ctrlKey && e.key === 'c' && self.term.hasSelection()) {
          document.execCommand('copy')
          return false
        }
      })

      const wheelSupport = 'onwheel' in document.createElement('div') ? 'wheel' : document.onmousewheel !== undefined ? 'mousewheel' : 'DOMMouseScroll'
      termWeb.addEventListener(wheelSupport, (e) => {
        if (e.ctrlKey) {
          e.preventDefault()
          if (e.deltaY < 0) {
            self.term.setOption('fontSize', ++this.fontSize)
          } else {
            self.term.setOption('fontSize', --this.fontSize)
          }
          try {
            fitAddon.fit()
          } catch (e) {
            // Ignore transient fit errors during resize.
          }
          if (self.ws !== null && self.ws.readyState === 1) {
            self.ws.send(`resize:${self.term.rows}:${self.term.cols}`)
          }
        }
      })

      window.addEventListener('resize', () => {
        try {
          fitAddon.fit()
        } catch (e) {
          // Ignore transient fit errors during resize.
        }
        if (self.ws !== null && self.ws.readyState === 1) {
          self.ws.send(`resize:${self.term.rows}:${self.term.cols}`)
        }
      })
    },
    async connected () {
      const sshInfo = this.$store.state.sshInfo
      this.ssh = Object.assign({}, sshInfo)

      const result = await checkSSH(this.$store.getters.sshReq)
      if (result.Msg !== 'success') {
        return
      }
      this.savePass = result.Data.savePass

      document.title = sshInfo.hostname
      let sshList = this.$store.state.sshList
      if (sshList === null) {
        if (this.savePass) {
          sshList = `[{"hostname": "${sshInfo.hostname}", "username": "${sshInfo.username}", "port":${sshInfo.port}, "logintype":${sshInfo.logintype}, "password":"${sshInfo.password}"}]`
        } else {
          sshList = `[{"hostname": "${sshInfo.hostname}", "username": "${sshInfo.username}", "port":${sshInfo.port},  "logintype":${sshInfo.logintype}}]`
        }
      } else {
        const sshListObj = JSON.parse(window.atob(sshList))
        sshListObj.forEach((v, i) => {
          if (v.hostname === sshInfo.hostname) {
            sshListObj.splice(i, 1)
          }
        })
        sshListObj.push({
          hostname: sshInfo.hostname,
          username: sshInfo.username,
          port: sshInfo.port,
          logintype: sshInfo.logintype
        })
        if (this.savePass) {
          sshListObj[sshListObj.length - 1].password = sshInfo.password
        }
        sshList = JSON.stringify(sshListObj)
      }

      this.$store.commit('SET_LIST', window.btoa(sshList))
    },
    close () {
      if (this.ws !== null) {
        this.ws.close()
        this.resetClose = true
      }
      if (this.term !== null) {
        this.term.dispose()
      }
    }
  },
  beforeDestroy () {
    window.removeEventListener('resize', this.handleWindowResize)
    this.stopResizeSftp()
    this.close()
  }
}
</script>

<style scoped>
.terminal-page-wrapper {
  display: flex;
  flex-direction: column;
  flex-grow: 1;
  min-height: 0;
  background: var(--card-bg);
  box-shadow: var(--shadow);
}

.terminal-page-container {
  display: flex;
  flex-grow: 1;
  min-height: 0;
  overflow: hidden;
  background: #f7f8fa;
}

.terminal-area {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  background-color: black;
}

#xterm-container {
  flex-grow: 1;
  width: 100%;
  padding-left: 2px;
}

.sftp-resizer {
  width: 6px;
  cursor: col-resize;
  position: relative;
  flex-shrink: 0;
  background: #f7f8fa;
}

.sftp-resizer::after {
  content: '';
  position: absolute;
  top: 0;
  bottom: 0;
  left: 2px;
  width: 2px;
  background: #dcdfe6;
}

.file-tree {
  width: 350px;
  border-left: 1px solid var(--input-border);
  background: #ffffff;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.terminal-footer {
  width: 100%;
  margin-left: -3rem;
  text-align: center;
  padding: 8px 0 6px;
  font-size: 15px;
  color: #0e0e0e;
  background: var(--card-bg);
  border-top: 1px solid var(--input-border);
  display: flex;
  justify-content: center;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.sftp-toggle-btn {
  display: none;
  background: none;
  border: none;
  color: #0e0e0e;
  font-size: 18px;
  cursor: pointer;
  padding: 0;
  line-height: 1;
}

.sftp-toggle-btn:hover {
  color: var(--text-primary);
}

@media (max-width: 768px) {
  .sftp-resizer {
    display: none;
  }

  .sftp-toggle-btn {
    display: inline-block;
  }

  .terminal-page-container {
    position: relative;
    overflow: hidden;
  }

  .file-tree {
    position: absolute;
    top: 0;
    right: 0;
    bottom: 0;
    width: 85%;
    max-width: 350px;
    transform: translateX(100%);
    transition: transform 0.3s ease-in-out;
    z-index: 20;
    border-left: none;
    box-shadow: -2px 0 10px rgba(0, 0, 0, 0.15);
  }

  .file-tree.is-visible {
    transform: translateX(0);
  }

  .terminal-footer {
    margin-left: 0;
    width: 100%;
  }
}
</style>
