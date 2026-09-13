import { describe, expect, it } from 'vitest';
import type { TaskTypeVO } from '../api/types';
import { filterCampaignByName, groupTaskTypes } from './menu';

/** 构造最小可用的 TaskTypeVO */
function makeType(overrides: Partial<TaskTypeVO> = {}): TaskTypeVO {
  return {
    id: 1,
    name: 'default',
    display_name: '默认类型',
    engine_mode: 'classic',
    is_active: true,
    is_campaign: false,
    ...overrides,
  };
}

describe('groupTaskTypes', () => {
  it('将 is_campaign=true 的类型归入专项组，其余归入普通组', () => {
    const campaign = makeType({ id: 1, is_campaign: true });
    const normal = makeType({ id: 2, is_campaign: false });
    const { campaign: campaigns, normal: normals } = groupTaskTypes([normal, campaign]);
    expect(campaigns).toEqual([campaign]);
    expect(normals).toEqual([normal]);
  });

  it('空列表返回两个空分组', () => {
    const { campaign, normal } = groupTaskTypes([]);
    expect(campaign).toHaveLength(0);
    expect(normal).toHaveLength(0);
  });

  it('保持输入顺序，不改变相对次序', () => {
    const a = makeType({ id: 3, is_campaign: true });
    const b = makeType({ id: 4, is_campaign: true });
    const c = makeType({ id: 5, is_campaign: false });
    const { campaign, normal } = groupTaskTypes([c, b, a]);
    expect(campaign.map((t) => t.id)).toEqual([4, 3]);
    expect(normal.map((t) => t.id)).toEqual([5]);
  });
});

describe('filterCampaignByName', () => {
  it('按 name 或 display_name 匹配专项类型', () => {
    const t = makeType({
      id: 7,
      name: 'owasp-top10',
      display_name: 'OWASP Top 10',
      is_campaign: true,
    });
    expect(filterCampaignByName([t], 'owasp-top10')?.id).toBe(7);
    expect(filterCampaignByName([t], 'OWASP Top 10')?.id).toBe(7);
    expect(filterCampaignByName([t], '不存在')).toBeUndefined();
  });
});