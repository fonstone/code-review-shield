import { getTaskTypes } from '../api/client';
import type { TaskTypeVO } from '../api/types';

/** sessionStorage 缓存键 */
const CACHE_KEY = 'code-shield-menu';
/** 缓存有效期：5 分钟 */
const CACHE_TTL_MS = 5 * 60 * 1000;

/** 侧边菜单单项 */
export interface MenuItem {
  key: number;
  name: string;
  display_name: string;
}

/** 侧边菜单分组 */
export interface MenuGroup {
  group: string;
  items: MenuItem[];
}

/** 缓存数据结构：数据 + 时间戳 */
interface MenuCache {
  timestamp: number;
  groups: MenuGroup[];
}

export const CAMPAIGN_GROUP = '专项扫描';
export const NORMAL_GROUP = '普通扫描';

/**
 * 按 is_campaign 将任务类型分为「专项扫描 / 普通扫描」两组（纯函数，便于单测）。
 */
export function groupTaskTypes(
  types: TaskTypeVO[],
): { campaign: TaskTypeVO[]; normal: TaskTypeVO[] } {
  const campaign: TaskTypeVO[] = [];
  const normal: TaskTypeVO[] = [];
  for (const type of types) {
    if (type.is_campaign) campaign.push(type);
    else normal.push(type);
  }
  return { campaign, normal };
}

/**
 * 按 name（或 display_name）精确查找专项任务类型，供 /campaign/:name 页使用。
 */
export function filterCampaignByName(
  types: TaskTypeVO[],
  name: string,
): TaskTypeVO | undefined {
  return types.find((t) => t.name === name || t.display_name === name);
}

function toMenuItem(type: TaskTypeVO): MenuItem {
  return { key: type.id, name: type.name, display_name: type.display_name };
}

/** 读取缓存；不存在或已过期（>5 分钟）时返回 null */
function readCache(): MenuGroup[] | null {
  try {
    const raw = sessionStorage.getItem(CACHE_KEY);
    if (!raw) return null;
    const cache = JSON.parse(raw) as MenuCache;
    if (!Array.isArray(cache.groups) || Date.now() - cache.timestamp > CACHE_TTL_MS) {
      sessionStorage.removeItem(CACHE_KEY);
      return null;
    }
    return cache.groups;
  } catch {
    return null;
  }
}

function writeCache(groups: MenuGroup[]): void {
  const cache: MenuCache = { timestamp: Date.now(), groups };
  sessionStorage.setItem(CACHE_KEY, JSON.stringify(cache));
}

/**
 * 构建动态侧边菜单：缓存优先（5 分钟过期重拉），未命中则调用
 * getTaskTypes(true) 拉取活跃任务类型并按专项/普通分组后写入缓存。
 */
export async function buildMenuItems(): Promise<MenuGroup[]> {
  const cached = readCache();
  if (cached) return cached;

  const types = await getTaskTypes(true);
  const { campaign, normal } = groupTaskTypes(types);
  const groups: MenuGroup[] = [];
  if (campaign.length > 0) {
    groups.push({ group: CAMPAIGN_GROUP, items: campaign.map(toMenuItem) });
  }
  if (normal.length > 0) {
    groups.push({ group: NORMAL_GROUP, items: normal.map(toMenuItem) });
  }
  writeCache(groups);
  return groups;
}

/** 读取菜单缓存（可能为 null） */
export function getMenuFromCache(): MenuGroup[] | null {
  return readCache();
}