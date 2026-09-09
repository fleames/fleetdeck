"use client";

import ReactECharts from "echarts-for-react";

const muted = "#8b97a8";
const legend = "#c3ccd8";
const split = "#2a3340";

/** Theme-aware ECharts defaults shared by metric history charts. */
export function chartThemeBase(): Record<string, unknown> {
  return {
    backgroundColor: "transparent",
    textStyle: { color: muted },
    tooltip: { trigger: "axis" },
    legend: { textStyle: { color: legend } },
    grid: { left: 48, right: 24, top: 40, bottom: 32 },
  };
}

export function MetricsChart({
  option,
  height = 280,
  className,
}: {
  option: object;
  height?: number;
  className?: string;
}) {
  const base = chartThemeBase();
  const opt = option as Record<string, unknown>;
  const merged = {
    ...base,
    ...opt,
    legend: { ...(base.legend as object), ...(opt.legend as object) },
    grid: { ...(base.grid as object), ...(opt.grid as object) },
  };
  return (
    <div className={className}>
      <ReactECharts option={merged} style={{ height }} opts={{ renderer: "canvas" }} />
    </div>
  );
}

export const chartAxisStyle = {
  axisLabel: { color: muted },
  splitLine: { lineStyle: { color: split } },
};
