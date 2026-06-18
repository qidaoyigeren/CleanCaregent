import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
  Area,
  AreaChart,
} from 'recharts';

interface DataPoint {
  time: string;
  value: number;
  label?: string;
}

interface MetricsChartProps {
  title: string;
  data: DataPoint[];
  color?: string;
  unit?: string;
  height?: number;
  type?: 'line' | 'area';
}

export default function MetricsChart({
  title,
  data,
  color = '#2563eb',
  unit = '',
  height = 200,
  type = 'area',
}: MetricsChartProps) {
  if (data.length === 0) {
    return (
      <div className="metrics-chart">
        <h4 className="metrics-chart__title">{title}</h4>
        <div className="metrics-chart__empty">暂无数据</div>
      </div>
    );
  }

  const ChartComponent = type === 'area' ? AreaChart : LineChart;
  const DataComponent = type === 'area' ? Area : Line;

  return (
    <div className="metrics-chart">
      <h4 className="metrics-chart__title">{title}</h4>
      <div className="metrics-chart__container">
        <ResponsiveContainer width="100%" height={height}>
          <ChartComponent data={data}>
            <CartesianGrid strokeDasharray="3 3" stroke="var(--color-border)" />
            <XAxis
              dataKey="time"
              tick={{ fontSize: 12, fill: 'var(--color-text-muted)' }}
              tickLine={false}
            />
            <YAxis
              tick={{ fontSize: 12, fill: 'var(--color-text-muted)' }}
              tickLine={false}
              axisLine={false}
              unit={unit}
            />
            <Tooltip
              contentStyle={{
                backgroundColor: 'var(--color-surface)',
                border: '1px solid var(--color-border)',
                borderRadius: 'var(--radius-md)',
                fontSize: 'var(--text-sm)',
              }}
              labelStyle={{ color: 'var(--color-text)' }}
            />
            <DataComponent
              type="monotone"
              dataKey="value"
              stroke={color}
              fill={color}
              fillOpacity={type === 'area' ? 0.1 : 0}
              strokeWidth={2}
              dot={false}
              activeDot={{ r: 4, fill: color }}
            />
          </ChartComponent>
        </ResponsiveContainer>
      </div>
    </div>
  );
}
