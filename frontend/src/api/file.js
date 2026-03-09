import request from '@/utils/request'
export function fileList(path, sshInfo) {
    return request.get(`/file/list?path=${path}&sshInfo=${sshInfo}`)
}

export function fileDelete(path, sshInfo) {
    const data = new URLSearchParams()
    data.append('path', path)
    data.append('sshInfo', sshInfo)
    return request.post('/file/delete', data)
}

export function fileCopy(path, sshInfo) {
    const data = new URLSearchParams()
    data.append('path', path)
    data.append('sshInfo', sshInfo)
    return request.post('/file/copy', data)
}

export function filePaste(srcPath, dstPath, sshInfo) {
    const data = new URLSearchParams()
    data.append('srcPath', srcPath)
    data.append('dstPath', dstPath)
    data.append('sshInfo', sshInfo)
    return request.post('/file/paste', data)
}
