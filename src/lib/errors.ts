import { strings } from "./strings";

/** errorText turns the engine's code and message into a sentence. */
export function errorText(code: string, message: string): string {
  switch (code) {
    case "E_DNS":
      return "无法解析域名，请检查网络或链接";
    case "E_NET":
      return "网络中断，稍后重试";
    case "E_TLS":
      return "TLS 证书校验失败，可能是服务器配置或代理问题";
    case "E_TIMEOUT":
      return "连接超时";
    case "E_HTTP":
      return message || "服务器拒绝了请求";
    case "E_AUTH":
      return "需要登录或授权，请用浏览器下载，或补上 Cookie";
    case "E_RETRY":
      return "服务器繁忙，重试次数已用完";
    case "E_DISK_FULL":
      return "磁盘空间不足";
    case "E_DISK":
      return "写入文件失败，请检查目录权限或磁盘";
    case "E_RANGE":
      return "服务器不支持该文件的分段下载";
    case "E_CHANGED":
      return "文件在下载过程中被修改，已停止";
    case "E_ENCODING":
      return "服务器返回了压缩内容，无法分段下载";
    case "E_RENAME":
      return "文件下载完成但无法重命名，请检查目标目录";
    case "E_CANCELED":
      return "已取消";
    case "E_INTERNAL":
      return message || strings.unknownError;
    default:
      return message || strings.unknownError;
  }
}
