import { useCallback, useMemo, useState } from 'react';
import { TOKEN_KEY, login as loginApi } from '../api/client';
import type { AuthResultVO } from '../api/types';

/** 当前用户信息在 localStorage 中的存储键 */
const USER_KEY = 'code-shield-user';

/** 从 localStorage 恢复已登录用户 */
function readStoredUser(): AuthResultVO | null {
  try {
    const raw = localStorage.getItem(USER_KEY);
    return raw ? (JSON.parse(raw) as AuthResultVO) : null;
  } catch {
    return null;
  }
}

export interface UseAuthResult {
  /** 当前登录用户（含 role），未登录为 null */
  user: AuthResultVO | null;
  /** 是否管理员角色 */
  isAdmin: boolean;
  /** 登录：成功后持久化 token 与用户信息 */
  login: (username: string, password: string) => Promise<AuthResultVO>;
  /** 登出：清除本地凭证并跳转登录页 */
  logout: () => void;
}

/** 登录态 Hook：token 持久化于 localStorage，登录/登出/当前用户（含 role） */
export function useAuth(): UseAuthResult {
  const [user, setUser] = useState<AuthResultVO | null>(readStoredUser);

  const login = useCallback(async (username: string, password: string) => {
    const result = await loginApi(username, password);
    localStorage.setItem(TOKEN_KEY, result.token);
    localStorage.setItem(USER_KEY, JSON.stringify(result));
    setUser(result);
    return result;
  }, []);

  const logout = useCallback(() => {
    localStorage.removeItem(TOKEN_KEY);
    localStorage.removeItem(USER_KEY);
    setUser(null);
    window.location.href = '/login';
  }, []);

  const isAdmin = useMemo(() => user?.role === 'admin', [user]);

  return { user, isAdmin, login, logout };
}