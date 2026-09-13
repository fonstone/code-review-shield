import { message } from 'antd';

/**
 * 复制文本到剪贴板：优先使用 Clipboard API，失败时降级为隐藏 textarea + execCommand。
 * @param text 待复制文本
 * @param tip 成功提示文案
 */
export async function copyText(text: string, tip = '已复制到剪贴板'): Promise<void> {
  try {
    await navigator.clipboard.writeText(text);
    message.success(tip);
  } catch {
    const textarea = document.createElement('textarea');
    textarea.value = text;
    textarea.style.position = 'fixed';
    textarea.style.opacity = '0';
    document.body.appendChild(textarea);
    textarea.select();
    document.execCommand('copy');
    document.body.removeChild(textarea);
    message.success(tip);
  }
}