import { useEffect, useRef } from 'react';
import * as echarts from 'echarts';

/**
 * ECharts 封装：容器挂载后初始化实例，deps 变化时重设 option，组件卸载时销毁实例。
 * @param option 图表配置；为 null 时跳过渲染（用于空数据场景）
 * @param deps 触发 setOption 的依赖数组
 */
export function useECharts(
  option: echarts.EChartsOption | null,
  deps: unknown[],
): React.RefObject<HTMLDivElement> {
  const containerRef = useRef<HTMLDivElement>(null);
  const chartRef = useRef<echarts.ECharts | null>(null);

  // 初始化与销毁
  useEffect(() => {
    if (!containerRef.current) return;
    const chart = echarts.init(containerRef.current);
    chartRef.current = chart;
    const onResize = () => chart.resize();
    window.addEventListener('resize', onResize);
    return () => {
      window.removeEventListener('resize', onResize);
      chart.dispose();
      chartRef.current = null;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // option 变化时更新
  useEffect(() => {
    if (chartRef.current && option) {
      chartRef.current.setOption(option);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);

  return containerRef;
}