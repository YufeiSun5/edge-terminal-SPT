import { useMemo } from "react";
import type { ReactNode } from "react";
import { Alert, Badge, Space, Table, Tag, Typography } from "antd";
import type { TableProps } from "antd";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  Activity,
  Database,
  Server,
  ShieldCheck,
  Wifi,
  WifiOff,
} from "lucide-react";
import type { TagSnapshot } from "@/shared/api/types";
import {
  getActiveDetectionRuns,
  getGateways,
  getHealth,
  getRealtimeVariables,
} from "./api";
import {
  getSidecarStatus,
  type SidecarState,
} from "@/shared/desktop/desktopBridge";

const sidecarTagColor: Record<SidecarState, string> = {
  online: "success",
  starting: "processing",
  unhealthy: "warning",
  stopped: "default",
  missing: "error",
  offline: "error",
  failed: "error",
  unavailable: "default",
};

export function EdgeStatusPage() {
  const { t } = useTranslation();

  const sidecarQuery = useQuery({
    queryKey: ["desktop", "sidecar"],
    queryFn: getSidecarStatus,
    refetchInterval: 2000,
  });

  const healthQuery = useQuery({
    queryKey: ["edge", "health"],
    queryFn: getHealth,
    refetchInterval: 3000,
    retry: false,
  });

  const gatewaysQuery = useQuery({
    queryKey: ["edge", "gateways"],
    queryFn: getGateways,
    refetchInterval: 5000,
    retry: false,
  });

  const variablesQuery = useQuery({
    queryKey: ["edge", "realtime-variables"],
    queryFn: () => getRealtimeVariables(),
    refetchInterval: 2000,
    retry: false,
  });

  const activeRunsQuery = useQuery({
    queryKey: ["edge", "active-runs"],
    queryFn: getActiveDetectionRuns,
    refetchInterval: 3000,
    retry: false,
  });

  const gatewayStatus = healthQuery.data?.gateways ?? gatewaysQuery.data ?? {};
  const gatewayEntries = Object.entries(gatewayStatus);
  const onlineGateways = gatewayEntries.filter(
    ([, status]) => status.active,
  ).length;
  const channels = healthQuery.data?.channels;
  const queueDepth = channels
    ? channels.logic + channels.discovery + channels.store
    : 0;
  const sidecarState = sidecarQuery.data?.state ?? "offline";
  const variables = variablesQuery.data ?? [];
  const activeRuns = activeRunsQuery.data ?? [];

  const tableColumns = useMemo<TableProps<TagSnapshot>["columns"]>(
    () => [
      {
        title: t("table.variable"),
        dataIndex: "display_name",
        key: "display_name",
        render: (_, row) => row.display_name || row.var_name,
      },
      {
        title: t("table.value"),
        key: "value",
        render: (_, row) => (
          <span className="mono">
            {row.is_string
              ? row.str_value || t("messages.noData")
              : row.value.toFixed(2)}
          </span>
        ),
      },
      {
        title: t("table.device"),
        dataIndex: "device_code",
        key: "device_code",
        render: (value) => value || "-",
      },
      {
        title: t("table.group"),
        dataIndex: "var_group",
        key: "var_group",
        render: (value) => value || "-",
      },
      {
        title: t("table.gateway"),
        dataIndex: "gateway_id",
        key: "gateway_id",
        width: 90,
      },
      {
        title: t("table.quality"),
        dataIndex: "quality",
        key: "quality",
        width: 92,
        render: (quality) => (
          <Tag color={quality === 1 ? "success" : "warning"}>
            {quality === 1 ? t("status.normal") : t("status.warning")}
          </Tag>
        ),
      },
    ],
    [t],
  );

  return (
    <div className="app-content edge-status-page">
      <div className="history-ambient-background" aria-hidden="true">
        <div className="history-orb history-orb-1" />
        <div className="history-orb history-orb-2" />
        <div className="history-orb history-orb-3" />
        <div className="history-noise" />
      </div>
      {sidecarState === "unavailable" && (
        <Alert
          className="compact-alert"
          type="info"
          showIcon
          message={t("messages.bridgeUnavailable")}
        />
      )}
      {sidecarState === "missing" && (
        <Alert
          className="compact-alert"
          type="error"
          showIcon
          message={t("messages.sidecarMissing")}
        />
      )}
      {healthQuery.isError && (
        <Alert
          className="compact-alert"
          type="warning"
          showIcon
          message={t("messages.backendOffline")}
        />
      )}

      <section className="status-grid edge-status-grid" aria-label={t("panels.edgeOverview")}>
        <StatusTile
          icon={<Server size={17} />}
          label={t("metrics.sidecar")}
          value={t(`status.${sidecarState}`)}
          tag={
            <Tag color={sidecarTagColor[sidecarState]}>
              {t(`status.${sidecarState}`)}
            </Tag>
          }
          note={
            sidecarQuery.data?.pid
              ? `PID ${sidecarQuery.data.pid}`
              : sidecarQuery.data?.error || ""
          }
        />
        <StatusTile
          icon={healthQuery.data ? <Wifi size={17} /> : <WifiOff size={17} />}
          label={t("metrics.api")}
          value={healthQuery.data?.status ?? t("status.notReady")}
          tag={<Badge status={healthQuery.data ? "success" : "error"} />}
          note="127.0.0.1:18080"
        />
        <StatusTile
          icon={<Activity size={17} />}
          label={t("metrics.tags")}
          value={String(healthQuery.data?.tags ?? variables.length)}
          note={t("panels.variables")}
        />
        <StatusTile
          icon={<ShieldCheck size={17} />}
          label={t("metrics.gateways")}
          value={`${onlineGateways}/${gatewayEntries.length}`}
          note={t("panels.gateways")}
        />
        <StatusTile
          icon={<Database size={17} />}
          label={t("metrics.activeRuns")}
          value={String(activeRuns.length)}
          note={t("panels.activeRuns")}
        />
      </section>

      <section className="main-grid">
        <div className="panel edge-glass-panel">
          <div className="panel-header">
            <Typography.Title level={2} className="panel-title">
              {t("panels.variables")}
            </Typography.Title>
            <Tag color={variablesQuery.isFetching ? "processing" : "default"}>
              {variablesQuery.isFetching
                ? t("actions.refresh")
                : `${variables.length}`}
            </Tag>
          </div>
          <div className="panel-body">
            <Table
              rowKey="var_id"
              size="small"
              columns={tableColumns}
              dataSource={variables.slice(0, 80)}
              pagination={{ pageSize: 12, size: "small" }}
              locale={{ emptyText: t("messages.noData") }}
              scroll={{ x: 780 }}
            />
          </div>
        </div>

        <div className="side-stack">
          <div className="panel edge-glass-panel">
            <div className="panel-header">
              <Typography.Title level={2} className="panel-title">
                {t("panels.channels")}
              </Typography.Title>
            </div>
            <div className="panel-body">
              <div className="metric-row">
                <Metric label="logic" value={String(channels?.logic ?? 0)} />
                <Metric
                  label="discovery"
                  value={String(channels?.discovery ?? 0)}
                />
                <Metric label="store" value={String(channels?.store ?? 0)} />
              </div>
              <div className="metric" style={{ marginTop: 10 }}>
                <div className="metric-label">{t("metrics.queueDepth")}</div>
                <div className="metric-value">{queueDepth}</div>
              </div>
            </div>
          </div>

          <div className="panel edge-glass-panel">
            <div className="panel-header">
              <Typography.Title level={2} className="panel-title">
                {t("panels.gateways")}
              </Typography.Title>
            </div>
            <div className="panel-body">
              <Space wrap>
                {gatewayEntries.length === 0 && (
                  <Tag>{t("messages.noData")}</Tag>
                )}
                {gatewayEntries.map(([gatewayId, status]) => (
                  <Tag
                    key={gatewayId}
                    color={status.active ? "success" : "error"}
                  >
                    {gatewayId}:{" "}
                    {status.active ? t("status.online") : t("status.offline")}
                  </Tag>
                ))}
              </Space>
            </div>
          </div>

          <div className="panel edge-glass-panel">
            <div className="panel-header">
              <Typography.Title level={2} className="panel-title">
                {t("panels.integration")}
              </Typography.Title>
            </div>
            <div className="panel-body">
              <Alert type="info" showIcon message={t("messages.authPending")} />
              <Alert
                style={{ marginTop: 10 }}
                type="warning"
                showIcon
                message={t("messages.ssoPending")}
              />
            </div>
          </div>
        </div>
      </section>
    </div>
  );
}

function StatusTile({
  icon,
  label,
  value,
  note,
  tag,
}: {
  icon: ReactNode;
  label: string;
  value: string;
  note?: string;
  tag?: ReactNode;
}) {
  return (
    <div className="status-tile">
      <div className="status-tile-label">
        <Space size={6}>
          {icon}
          <span>{label}</span>
        </Space>
        {tag}
      </div>
      <div className="status-tile-value">{value}</div>
      <div className="status-tile-note">{note}</div>
    </div>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="metric">
      <div className="metric-label">{label}</div>
      <div className="metric-value">{value}</div>
    </div>
  );
}
