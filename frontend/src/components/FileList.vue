<template>
  <div
    class="file-list-wrapper"
    @dragenter.prevent="handlePanelDragEnter"
    @dragover.prevent="handlePanelDragOver"
    @dragleave.prevent="handlePanelDragLeave"
    @drop.prevent="handlePanelDrop"
  >
    <div class="sftp-title">SFTP文件管理</div>
    <div class="file-header">
      <el-input
        class="path-input"
        v-model="currentPath"
        size="small"
        @keyup.enter.native="getFileList()"
        @blur="getFileList"
        placeholder="当前路径..."
      ></el-input>
      <el-button-group>
        <el-button type="primary" size="small" icon="el-icon-s-home" @click="goToHome" title="主目录"></el-button>
        <el-button type="primary" size="small" icon="el-icon-arrow-up" @click="upDirectory" title="返回上级目录"></el-button>
        <el-button type="primary" size="small" icon="el-icon-refresh" @click="getFileList" title="刷新当前目录"></el-button>
        <el-dropdown @click="openUploadDialog" @command="handleUploadCommand" size="small">
          <el-button type="primary" size="small" icon="el-icon-upload"></el-button>
          <el-dropdown-menu slot="dropdown">
            <el-dropdown-item command="file">{{ $t('uploadFile') }}</el-dropdown-item>
            <el-dropdown-item command="folder">{{ $t('uploadFolder') }}</el-dropdown-item>
          </el-dropdown-menu>
        </el-dropdown>
      </el-button-group>
    </div>

    <el-dialog custom-class="uploadContainer" :title="$t(titleTip)" :visible.sync="uploadVisible" append-to-body width="32%">
      <div
        class="dialog-drop-zone"
        @dragenter.prevent="handleDialogDragEnter"
        @dragover.prevent="handleDialogDragOver"
        @dragleave.prevent="handleDialogDragLeave"
        @drop.prevent="handleDialogDrop"
      >
        <el-upload
          ref="upload"
          multiple
          drag
          :show-file-list="false"
          :http-request="enqueueUploadRequest"
          :action="uploadUrl"
          :data="uploadData"
          :before-upload="beforeUpload"
          :on-progress="uploadProgress"
          :on-error="uploadError"
          :on-success="uploadSuccess"
        >
          <i class="el-icon-upload"></i>
          <div class="el-upload__text">{{ $t(selectTip) }}</div>
        </el-upload>

        <div v-if="isDialogDragOver" class="drop-mask dialog-mask">
          <div class="drop-mask-content">拖拽文件或文件夹到这里上传</div>
        </div>
      </div>
    </el-dialog>

    <el-table :data="fileList" class="file-table" @row-click="rowClick" height="100%" stripe border>
      <el-table-column :label="$t('Name')" width="260" :resizable="true" sortable :sort-method="nameSort">
        <template slot-scope="scope">
          <div class="name-cell" :class="{ 'is-dir': scope.row.IsDir }" :title="scope.row.Name">
            <i :class="scope.row.IsDir ? 'el-icon-folder' : 'el-icon-document'"></i>
            <span class="name-text">{{ scope.row.Name }}</span>
          </div>
        </template>
      </el-table-column>
      <el-table-column :label="$t('Size')" prop="Size" width="90" :resizable="true"></el-table-column>
      <el-table-column :label="$t('ModifiedTime')" prop="ModifyTime" min-width="160" sortable show-overflow-tooltip :resizable="true"></el-table-column>
    </el-table>

    <div v-if="isPanelDragOver" class="drop-mask">
      <div class="drop-mask-content">拖拽文件或文件夹到这里上传到当前目录</div>
    </div>

    <div v-if="uploadTasks.length" class="task-panel">
      <div class="task-panel-header">
        <span class="task-summary">上传任务：等待 {{ queuedCount }}，上传中 {{ uploadingCount }}，失败 {{ failedCount }}，成功累计 {{ successTotalCount }}</span>
        <div class="task-panel-actions">
          <span class="concurrency-label">并发</span>
          <el-button-group class="concurrency-group">
            <el-button size="mini" icon="el-icon-minus" @click="decrementConcurrent" />
            <el-button size="mini" class="concurrency-value" @click="resetConcurrent">{{ maxConcurrentUploads }}</el-button>
            <el-button size="mini" icon="el-icon-plus" @click="incrementConcurrent" />
          </el-button-group>
          <el-button type="text" @click="retryAllFailedTasks">重试失败</el-button>
          <el-button type="text" @click="clearFinishedTasks">清理已完成</el-button>
        </div>
      </div>
      <div class="task-list">
        <div class="task-item" v-for="task in taskDisplayList" :key="task.id">
          <div class="task-meta">
            <span class="task-name" :title="task.fullName">{{ task.fullName }}</span>
            <span class="task-status" :class="`is-${task.status}`">{{ taskStatusText(task.status) }}</span>
          </div>
          <el-progress
            :percentage="task.progress"
            :stroke-width="6"
            :status="task.status === 'failed' ? 'exception' : (task.status === 'success' ? 'success' : '')"
          />
          <div v-if="task.status === 'failed'" class="task-error-row">
            <div v-if="task.message" class="task-error">{{ task.message }}</div>
            <el-button type="text" size="mini" @click="retryTask(task)">重试</el-button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script>
import { fileList } from '@/api/file'
import request from '@/utils/request'
import { mapState } from 'vuex'

export default {
  name: 'FileList',
  data () {
    return {
      uploadVisible: false,
      fileList: [],
      downloadFilePath: '',
      currentPath: '/',
      selectTip: 'clickSelectFile',
      titleTip: 'uploadFile',
      uploadTip: '',
      progressPercent: 0,
      initialRedirectDone: false,
      homePath: '',
      uploadMode: 'file',
      isPanelDragOver: false,
      isDialogDragOver: false,
      panelDragCounter: 0,
      dialogDragCounter: 0,
      uploadTasks: [],
      uploadRequestQueue: [],
      activeUploadCount: 0,
      maxConcurrentUploads: 2,
      resumableThreshold: 8 * 1024 * 1024,
      resumableChunkSize: 5 * 1024 * 1024,
      successKeepLimit: 50,
      successTotalCount: 0,
      refreshTimer: null
    }
  },
  mounted () {
    if (!this.currentPath || this.currentPath === '/') {
      this.getFileList()
    }
  },
  computed: {
    ...mapState(['currentTab']),
    sshInfoReady () {
      return this.$store.state.sshInfo && this.$store.state.sshInfo.hostname
    },
    uploadUrl: () => {
      return `${process.env.NODE_ENV === 'production' ? `${location.origin}` : 'api'}/file/upload`
    },
    uploadData () {
      return {
        sshInfo: this.$store.getters.sshReq,
        path: this.currentPath
      }
    },
    taskDisplayList () {
      return this.uploadTasks.slice().reverse()
    },
    queuedCount () {
      return this.uploadTasks.filter(v => v.status === 'queued').length
    },
    uploadingCount () {
      return this.uploadTasks.filter(v => v.status === 'uploading').length
    },
    failedCount () {
      return this.uploadTasks.filter(v => v.status === 'failed').length
    }
  },
  watch: {
    sshInfoReady (newValue, oldValue) {
      if (newValue && !oldValue) {
        this.getFileList()
      }
    },
    currentTab () {
      this.fileList = []
      this.currentPath = this.currentTab && this.currentTab.path ? this.currentTab.path : '/'
    },
    maxConcurrentUploads () {
      this.processUploadQueue()
    }
  },
  beforeDestroy () {
    if (this.refreshTimer) {
      clearTimeout(this.refreshTimer)
      this.refreshTimer = null
    }
  },
  methods: {
    createUploadTask (file, dir = '', uid = '', keepFileRef = false) {
      const fullName = dir ? `${dir}/${file.name}` : file.name
      const task = {
        id: `${Date.now()}-${Math.random().toString(16).slice(2)}`,
        uid,
        file: keepFileRef ? file : null,
        dir,
        fullName,
        status: 'queued',
        progress: 0,
        message: ''
      }
      this.uploadTasks.push(task)
      return task
    },
    getUploadTaskByUid (uid) {
      return this.uploadTasks.find(v => v.uid === uid)
    },
    taskStatusText (status) {
      if (status === 'queued') return '等待上传'
      if (status === 'uploading') return '上传中'
      if (status === 'success') return '成功'
      if (status === 'failed') return '失败'
      return status
    },
    decrementConcurrent () {
      this.maxConcurrentUploads = Math.max(1, this.maxConcurrentUploads - 1)
    },
    incrementConcurrent () {
      this.maxConcurrentUploads = Math.min(10, this.maxConcurrentUploads + 1)
    },
    resetConcurrent () {
      this.maxConcurrentUploads = 2
    },
    retryTask (task) {
      if (!task || !task.file) {
        this.$message.warning('无法重试：未找到原始文件')
        return
      }
      task.status = 'queued'
      task.progress = 0
      task.message = ''
      this.enqueueTaskUpload(task.file, task.dir || '', task)
    },
    retryAllFailedTasks () {
      const failedTasks = this.uploadTasks.filter(v => v.status === 'failed')
      if (!failedTasks.length) {
        this.$message.info('没有可重试的失败任务')
        return
      }
      let queued = 0
      for (const task of failedTasks) {
        if (!task.file) {
          continue
        }
        task.status = 'queued'
        task.progress = 0
        task.message = ''
        this.enqueueTaskUpload(task.file, task.dir || '', task)
        queued += 1
      }
      this.$message.success(`已加入重试队列：${queued} 个任务`)
    },
    clearFinishedTasks () {
      this.uploadTasks = this.uploadTasks.filter(v => v.status === 'uploading' || v.status === 'queued')
      this.successTotalCount = 0
    },
    markTaskSuccess (task) {
      if (!task) return
      const wasSuccess = task.status === 'success'
      task.status = 'success'
      task.progress = 100
      task.message = ''
      task.file = null
      task.dir = ''
      if (!wasSuccess) {
        this.successTotalCount += 1
      }
      this.pruneSuccessTasks()
    },
    pruneSuccessTasks () {
      const keep = Math.max(0, Number(this.successKeepLimit) || 0)
      const successTasks = this.uploadTasks.filter(v => v.status === 'success')
      if (successTasks.length <= keep) {
        return
      }
      const keepIds = new Set(successTasks.slice(-keep).map(v => v.id))
      this.uploadTasks = this.uploadTasks.filter(v => v.status !== 'success' || keepIds.has(v.id))
    },
    handlePanelDragEnter () {
      this.panelDragCounter += 1
      this.isPanelDragOver = true
    },
    handlePanelDragOver () {
      this.isPanelDragOver = true
    },
    handlePanelDragLeave () {
      this.panelDragCounter = Math.max(0, this.panelDragCounter - 1)
      if (this.panelDragCounter === 0) {
        this.isPanelDragOver = false
      }
    },
    async handlePanelDrop (e) {
      this.panelDragCounter = 0
      this.isPanelDragOver = false
      await this.uploadFromDataTransfer(e.dataTransfer)
    },
    handleDialogDragEnter () {
      this.dialogDragCounter += 1
      this.isDialogDragOver = true
    },
    handleDialogDragOver () {
      this.isDialogDragOver = true
    },
    handleDialogDragLeave () {
      this.dialogDragCounter = Math.max(0, this.dialogDragCounter - 1)
      if (this.dialogDragCounter === 0) {
        this.isDialogDragOver = false
      }
    },
    async handleDialogDrop (e) {
      this.dialogDragCounter = 0
      this.isDialogDragOver = false
      if (this.uploadVisible) {
        this.uploadVisible = false
      }
      const result = await this.uploadFromDataTransfer(e.dataTransfer, {
        mode: this.uploadMode,
        strictMode: true
      })
      return result
    },
    async uploadFromDataTransfer (dataTransfer, options = {}) {
      const mode = options.mode || 'both'
      const strictMode = Boolean(options.strictMode)
      const entries = await this.collectDroppedEntries(dataTransfer)
      if (!entries.length) {
        this.$message.warning('未检测到可上传的文件或文件夹')
        return { successCount: 0, totalCount: 0 }
      }

      const hasFolder = entries.some(v => v.dir && v.dir.length > 0)
      if (strictMode && mode === 'file' && hasFolder) {
        this.$message.warning('当前是“上传文件”，请拖入文件；如需拖入文件夹请选择“上传文件夹”。')
        return { successCount: 0, totalCount: entries.length }
      }
      if (strictMode && mode === 'folder' && !hasFolder) {
        this.$message.warning('当前是“上传文件夹”，请拖入文件夹；如需拖入文件请选择“上传文件”。')
        return { successCount: 0, totalCount: entries.length }
      }

      const queue = entries.map(item => ({
        ...item,
        task: this.createUploadTask(item.file, item.dir || '')
      }))

      for (const item of queue) {
        this.enqueueTaskUpload(item.file, item.dir || '', item.task)
      }
      this.$message.success(`已加入上传队列：${queue.length} 个文件`)
      return { successCount: queue.length, totalCount: entries.length }
    },
    async collectDroppedEntries (dataTransfer) {
      const items = Array.from(dataTransfer && dataTransfer.items ? dataTransfer.items : [])
      const supportsEntry = items.some(v => typeof v.webkitGetAsEntry === 'function')

      if (supportsEntry) {
        let all = []
        for (const item of items) {
          const entry = item.webkitGetAsEntry ? item.webkitGetAsEntry() : null
          if (!entry) {
            continue
          }
          const children = await this.walkFileTree(entry, '')
          all = all.concat(children)
        }
        return all
      }

      const files = Array.from(dataTransfer && dataTransfer.files ? dataTransfer.files : [])
      return files
        .filter(file => file && file.name)
        .map(file => {
          const rel = file.webkitRelativePath || ''
          const slash = rel.lastIndexOf('/')
          const dir = slash > 0 ? rel.slice(0, slash) : ''
          return { file, dir }
        })
    },
    async walkFileTree (entry, parentPath) {
      if (entry.isFile) {
        const file = await this.getFileFromEntry(entry)
        if (!file) {
          return []
        }
        return [{ file, dir: parentPath }]
      }

      if (!entry.isDirectory) {
        return []
      }

      const currentPath = parentPath ? `${parentPath}/${entry.name}` : entry.name
      const children = await this.readAllEntries(entry)
      let all = []
      for (const child of children) {
        const childList = await this.walkFileTree(child, currentPath)
        all = all.concat(childList)
      }
      return all
    },
    getFileFromEntry (entry) {
      return new Promise(resolve => {
        entry.file(
          file => resolve(file),
          () => resolve(null)
        )
      })
    },
    readAllEntries (dirEntry) {
      return new Promise(resolve => {
        const reader = dirEntry.createReader()
        const entries = []
        const readBatch = () => {
          reader.readEntries(result => {
            if (!result.length) {
              resolve(entries)
              return
            }
            entries.push(...result)
            readBatch()
          }, () => resolve(entries))
        }
        readBatch()
      })
    },
    enqueueTaskUpload (file, dir = '', task = null) {
      return new Promise(resolve => {
        this.uploadRequestQueue.push({
          file,
          dir,
          task: task || this.createUploadTask(file, dir, '', false),
          resolve
        })
        this.processUploadQueue()
      })
    },
    goToHome () {
      if (this.homePath) {
        if (this.currentPath !== this.homePath) {
          this.currentPath = this.homePath
          this.getFileList()
        }
      } else {
        this.$message.warning('主目录信息尚不可用，请刷新重试')
      }
    },
    openUploadDialog () {
      this.uploadTip = ''
      this.uploadVisible = true
    },
    enqueueUploadRequest (option) {
      this.uploadRequestQueue.push(option)
      this.processUploadQueue()
    },
    processUploadQueue () {
      while (this.activeUploadCount < this.maxConcurrentUploads && this.uploadRequestQueue.length > 0) {
        const option = this.uploadRequestQueue.shift()
        this.activeUploadCount += 1
        this.executeUploadRequest(option).finally(() => {
          this.activeUploadCount = Math.max(0, this.activeUploadCount - 1)
          this.processUploadQueue()
        })
      }
    },
    buildUploadClientKey (file, dirPath = '') {
      return [
        file.name || '',
        file.size || 0,
        file.lastModified || 0,
        this.currentPath || '/',
        dirPath || ''
      ].join('::')
    },
    async executeDirectUpload (file, dirPath, task, option) {
      const formData = new FormData()
      formData.append('sshInfo', this.$store.getters.sshReq)
      formData.append('path', this.currentPath)
      formData.append('id', file.uid || `${Date.now()}-${Math.random().toString(16).slice(2)}`)
      if (dirPath) {
        formData.append('dir', dirPath)
      }
      formData.append('file', file, file.name)

      const result = await request.post('/file/upload', formData, {
        timeout: 120000,
        skipGlobalError: true,
        onUploadProgress: (evt) => {
          if (!evt || !evt.total) return
          const percent = Math.max(task.progress, Math.min(99, Math.round((evt.loaded / evt.total) * 100)))
          task.progress = percent
          if (option.onProgress) {
            option.onProgress({ percent })
          }
        }
      })
      return result
    },
    async executeResumableUpload (file, dirPath, task, option) {
      const chunkSize = this.resumableChunkSize
      const totalChunks = Math.max(1, Math.ceil(file.size / chunkSize))
      const initPayload = {
        clientKey: this.buildUploadClientKey(file, dirPath),
        sshInfo: this.$store.getters.sshReq,
        path: this.currentPath,
        dir: dirPath,
        fileName: file.name,
        fileSize: file.size,
        chunkSize,
        totalChunks
      }
      const initResp = await request.post('/file/upload/init', initPayload, {
        timeout: 30000,
        skipGlobalError: true
      })
      if (!initResp || initResp.Msg !== 'success') {
        throw new Error((initResp && initResp.Msg) || '初始化分片上传失败')
      }
      const fileId = initResp.Data && initResp.Data.fileId
      if (!fileId) {
        throw new Error('分片会话创建失败')
      }
      const uploadedSet = new Set((initResp.Data && initResp.Data.uploadedChunks) || [])
      let doneChunks = uploadedSet.size
      task.progress = Math.max(task.progress, Math.min(99, Math.round((doneChunks / totalChunks) * 100)))

      for (let i = 0; i < totalChunks; i++) {
        if (uploadedSet.has(i)) {
          continue
        }
        const start = i * chunkSize
        const end = Math.min(file.size, start + chunkSize)
        const chunkFile = file.slice(start, end)
        const chunkForm = new FormData()
        chunkForm.append('fileId', fileId)
        chunkForm.append('chunkIndex', String(i))
        chunkForm.append('file', chunkFile, `${file.name}.part${i}`)

        await request.post('/file/upload/chunk', chunkForm, {
          timeout: 120000,
          skipGlobalError: true,
          onUploadProgress: (evt) => {
            if (!evt || !evt.total) return
            const currentChunkPercent = Math.min(1, evt.loaded / evt.total)
            const overall = ((doneChunks + currentChunkPercent) / totalChunks) * 100
            const percent = Math.max(task.progress, Math.min(99, Math.round(overall)))
            task.progress = percent
            if (option.onProgress) {
              option.onProgress({ percent })
            }
          }
        })
        doneChunks += 1
        const percent = Math.max(task.progress, Math.min(99, Math.round((doneChunks / totalChunks) * 100)))
        task.progress = percent
      }

      const completeForm = new FormData()
      completeForm.append('fileId', fileId)
      const completeResp = await request.post('/file/upload/complete', completeForm, {
        timeout: 120000,
        skipGlobalError: true
      })
      return completeResp
    },
    async executeUploadRequest (option) {
      const file = option.file
      const dirPath = option.dir !== undefined
        ? option.dir
        : (file.webkitRelativePath ? file.webkitRelativePath.substring(0, file.webkitRelativePath.lastIndexOf('/')) : '')
      let task = option.task || this.getUploadTaskByUid(file.uid)
      if (!task) {
        task = this.createUploadTask(file, dirPath, file.uid || '', false)
      }
      task.status = 'uploading'

      try {
        const useResumable = file && file.size >= this.resumableThreshold
        const result = useResumable
          ? await this.executeResumableUpload(file, dirPath, task, option)
          : await this.executeDirectUpload(file, dirPath, task, option)
        if (result.Msg !== 'success') {
          task.status = 'failed'
          task.message = result.Msg || '未知错误'
          // Keep file ref only for failed tasks so retry can work.
          task.file = file
          task.dir = dirPath
          if (option.onError) option.onError(new Error(task.message), file)
          if (option.resolve) option.resolve(false)
          return
        }
        this.markTaskSuccess(task)
        if (option.onSuccess) option.onSuccess(result, file)
        if (option.resolve) option.resolve(true)
        this.scheduleFileListRefresh()
      } catch (err) {
        task.status = 'failed'
        task.message = (err && err.message) || '网络或服务异常'
        // Keep file ref only for failed tasks so retry can work.
        task.file = file
        task.dir = dirPath
        if (option.onError) option.onError(err, file)
        if (option.resolve) option.resolve(false)
      }
    },
    scheduleFileListRefresh () {
      if (this.refreshTimer) {
        clearTimeout(this.refreshTimer)
      }
      this.refreshTimer = setTimeout(() => {
        this.getFileList()
        this.refreshTimer = null
      }, 800)
    },
    handleUploadCommand (cmd) {
      this.uploadMode = cmd === 'folder' ? 'folder' : 'file'
      if (cmd === 'folder') {
        this.selectTip = 'clickSelectFolder'
        this.titleTip = 'uploadFolder'
      } else {
        this.selectTip = 'clickSelectFile'
        this.titleTip = 'uploadFile'
      }
      this.openUploadDialog()

      const isFolder = cmd === 'folder'
      const supported = this.webkitdirectorySupported()
      if (!supported) {
        if (isFolder) {
          this.$message.warning('当前浏览器不支持文件夹选择')
        }
        return
      }

      this.$nextTick(() => {
        const input = document.getElementsByClassName('el-upload__input')[0]
        if (input) {
          input.webkitdirectory = isFolder
        }
      })
    },
    webkitdirectorySupported () {
      return 'webkitdirectory' in document.createElement('input')
    },
    beforeUpload (file) {
      if (this.uploadVisible) {
        this.uploadVisible = false
      }
      this.uploadData.id = file.uid
      const dirPath = file.webkitRelativePath
      this.uploadData.dir = dirPath ? dirPath.substring(0, dirPath.lastIndexOf('/')) : ''
      if (!this.getUploadTaskByUid(file.uid)) {
        this.createUploadTask(file, this.uploadData.dir || '', file.uid, false)
      }
      return true
    },
    uploadSuccess (r, file) {
      const task = this.getUploadTaskByUid(file.uid)
      if (task) {
        if (r && r.Msg === 'success') {
          this.markTaskSuccess(task)
        } else {
          task.status = 'failed'
          task.message = (r && r.Msg) || '上传失败'
        }
      }
      this.scheduleFileListRefresh()
    },
    uploadError (err, file) {
      const task = this.getUploadTaskByUid(file && file.uid)
      if (task) {
        task.status = 'failed'
        task.message = '网络或服务异常'
      }
    },
    uploadProgress (e, f) {
      const task = this.getUploadTaskByUid(f.uid)
      if (!task) return
      task.status = 'uploading'
      task.progress = Math.max(task.progress, Math.min(99, Math.round(e.percent)))
    },
    nameSort (a, b) {
      return a.Name > b.Name
    },
    rowClick (row) {
      if (row.IsDir) {
        this.currentPath = this.currentPath.charAt(this.currentPath.length - 1) === '/'
          ? this.currentPath + row.Name
          : this.currentPath + '/' + row.Name
        this.getFileList()
      } else {
        this.downloadFilePath = this.currentPath.charAt(this.currentPath.length - 1) === '/'
          ? this.currentPath + row.Name
          : this.currentPath + '/' + row.Name
        this.downloadFile()
      }
    },
    async getFileList () {
      this.currentPath = this.currentPath.replace(/\\+/g, '/')
      if (this.currentPath === '') {
        this.currentPath = '/'
      }
      const result = await fileList(this.currentPath, this.$store.getters.sshReq)
      if (result.Msg === 'success') {
        if (result.Data.home) {
          this.homePath = result.Data.home
        }
        this.fileList = result.Data.list || []

        if (!this.initialRedirectDone && result.Data.home && result.Data.home !== '/' && this.currentPath !== result.Data.home) {
          this.initialRedirectDone = true
          this.currentPath = result.Data.home
          await this.getFileList()
        }
      } else {
        this.fileList = []
        this.$message({
          message: result.Msg,
          type: 'error',
          duration: 3000
        })
      }
    },
    upDirectory () {
      if (this.currentPath === '/') {
        return
      }
      let pathList = this.currentPath.split('/')
      if (pathList[pathList.length - 1] === '') {
        pathList = pathList.slice(0, pathList.length - 2)
      } else {
        pathList = pathList.slice(0, pathList.length - 1)
      }
      this.currentPath = pathList.length === 1 ? '/' : pathList.join('/')
      this.getFileList()
    },
    downloadFile () {
      const prefix = process.env.NODE_ENV === 'production' ? `${location.origin}` : 'api'
      const downloadUrl = `${prefix}/file/download?path=${this.downloadFilePath}&sshInfo=${this.$store.getters.sshReq}`
      window.open(downloadUrl)
    }
  }
}
</script>

<style lang="scss">
.file-list-wrapper {
  display: flex;
  flex-direction: column;
  height: 100%;
  width: 100%;
  min-width: 0;
  padding-top: 10px;
  box-sizing: border-box;
  background: #fff;
  position: relative;

  .sftp-title {
    font-size: 16px;
    font-weight: bold;
    color: var(--text-color);
    text-align: center;
    padding-bottom: 8px;
    margin-bottom: 8px;
    border-bottom: 1px solid var(--input-border);
    flex-shrink: 0;
  }

  .file-header {
    flex-shrink: 0;
    margin-bottom: 10px;
    display: flex;
    align-items: center;
  }

  .path-input {
    flex: 1;
    padding: 0 5px;
    margin-right: 2px;
  }

  .file-header .el-button-group .el-button {
    padding: 8px;
    width: 36px;
    height: 32px;
    line-height: 1;
  }

  .file-table {
    flex-grow: 1;
    min-height: 0;
    width: 100%;

    &.el-table th {
      height: 44px;
      padding: 0 6px;
    }

    &.el-table td {
      padding: 0 6px;
    }

    .cell {
      padding: 8px 2px;
      line-height: 20px;
    }

    th > .cell {
      display: flex;
      align-items: center;
    }

    .name-cell {
      display: flex;
      align-items: center;
      gap: 6px;
      min-width: 0;
      cursor: pointer;
    }

    .name-cell.is-dir {
      color: #0c60b5;
      font-weight: 500;
    }

    .name-text {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      display: inline-block;
      min-width: 0;
      flex: 1;
    }

    .el-table,
    .el-table__header-wrapper,
    .el-table__body-wrapper,
    .el-table__empty-block {
      background: #fff;
    }
  }
}

.dialog-drop-zone {
  position: relative;
}

.drop-mask {
  position: absolute;
  inset: 0;
  background: rgba(64, 158, 255, 0.12);
  border: 2px dashed #409eff;
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 30;
}

.dialog-mask {
  border-radius: 6px;
}

.drop-mask-content {
  background: #fff;
  color: #1f2937;
  border: 1px solid #bfdbfe;
  border-radius: 8px;
  padding: 10px 16px;
  font-size: 14px;
  font-weight: 600;
}

.task-panel {
  flex-shrink: 0;
  border-top: 1px solid #e5e7eb;
  background: #fafafa;
  padding: 8px 10px;
  max-height: 190px;
  overflow: auto;
}

.task-panel-header {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 6px;
  font-size: 13px;
  font-weight: 600;
  color: #374151;
  margin-bottom: 6px;
}

.task-summary {
  line-height: 1.4;
  word-break: break-all;
}

.task-panel-actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.concurrency-label {
  font-size: 12px;
  color: #6b7280;
}

.concurrency-group .concurrency-value {
  min-width: 38px;
  padding: 7px 8px;
}

.task-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.task-item {
  background: #fff;
  border: 1px solid #e5e7eb;
  border-radius: 6px;
  padding: 6px 8px;
}

.task-meta {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 4px;
  gap: 10px;
}

.task-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  min-width: 0;
  flex: 1;
  font-size: 12px;
  color: #374151;
}

.task-status {
  font-size: 12px;
  flex-shrink: 0;
}

.task-status.is-uploading {
  color: #2563eb;
}

.task-status.is-queued {
  color: #6b7280;
}

.task-status.is-success {
  color: #16a34a;
}

.task-status.is-failed {
  color: #dc2626;
}

.task-error {
  margin-top: 4px;
  color: #dc2626;
  font-size: 12px;
  line-height: 1.3;
}

.task-error-row {
  margin-top: 4px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.uploadContainer {
  .el-upload {
    display: flex;
  }

  .el-upload-dragger {
    width: 95%;
  }
}
</style>
