import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Save } from "lucide-react";
import { adminApiRequest } from "@/lib/api";
import { BarChart } from "@/components/charts/BarChart";
import { KPICard } from "@/components/shared/KPICard";
import { DataTable } from "@/components/shared/DataTable";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { useCreditsOverview, useUserCreditTransactions } from "@/hooks/use-credits";
import { useAnalyticsTimeseries } from "@/hooks/use-analytics";
import type { CreditTransaction } from "@/lib/types";
import { TimeAgo } from "@/components/shared/TimeAgo";

type BillingConfigPayload = {
  enabled: boolean;
  default_cost: number;
  zero_balance_action: string;
  method_multipliers: Record<string, number>;
};

const TRANSACTION_COLUMNS: ColumnDef<CreditTransaction>[] = [
  { accessorKey: "type", header: "Type" },
  { accessorKey: "amount", header: "Amount" },
  {
    id: "created_at",
    header: "When",
    cell: ({ row }) => <TimeAgo value={row.original.created_at} />,
  },
];

export function CreditsPage() {
  const queryClient = useQueryClient();
  const overviewQuery = useCreditsOverview();
  const timeseriesQuery = useAnalyticsTimeseries({ window: "24h", granularity: "1h" });

  const billingConfigQuery = useQuery({
    queryKey: ["billing-config"],
    queryFn: () => adminApiRequest<BillingConfigPayload>("/admin/api/v1/billing/config"),
  });

  const routeCostsQuery = useQuery({
    queryKey: ["billing-route-costs"],
    queryFn: () => adminApiRequest<{ route_costs: Record<string, number> }>("/admin/api/v1/billing/route-costs"),
  });

  const [selectedUserID, setSelectedUserID] = useState("");
  const transactionsQuery = useUserCreditTransactions(selectedUserID, { limit: 50 });

  const [methodMultipliers, setMethodMultipliers] = useState("{}");
  const [routeCosts, setRouteCosts] = useState("{}");

  useEffect(() => {
    if (billingConfigQuery.data?.method_multipliers) {
      setMethodMultipliers(JSON.stringify(billingConfigQuery.data.method_multipliers, null, 2));
    }
  }, [billingConfigQuery.data?.method_multipliers]);

  useEffect(() => {
    if (routeCostsQuery.data?.route_costs) {
      setRouteCosts(JSON.stringify(routeCostsQuery.data.route_costs, null, 2));
    }
  }, [routeCostsQuery.data?.route_costs]);

  useEffect(() => {
    if (!selectedUserID && overviewQuery.data?.top_consumers?.length) {
      setSelectedUserID(overviewQuery.data.top_consumers[0].user_id);
    }
  }, [overviewQuery.data?.top_consumers, selectedUserID]);

  const saveBillingMutation = useMutation({
    mutationFn: async () => {
      const parsed = JSON.parse(methodMultipliers) as Record<string, number>;
      const current = billingConfigQuery.data;
      if (!current) {
        return;
      }
      return adminApiRequest("/admin/api/v1/billing/config", {
        method: "PUT",
        body: {
          ...current,
          method_multipliers: parsed,
        },
      });
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["billing-config"] });
    },
  });

  const saveRouteCostsMutation = useMutation({
    mutationFn: async () => {
      const parsed = JSON.parse(routeCosts) as Record<string, number>;
      return adminApiRequest("/admin/api/v1/billing/route-costs", {
        method: "PUT",
        body: { route_costs: parsed },
      });
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["billing-route-costs"] });
    },
  });

  const chartData = useMemo(
    () =>
      (timeseriesQuery.data?.items ?? []).map((item) => ({
        name: new Date(item.timestamp).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }),
        value: item.credits_consumed,
        secondary: item.errors,
      })),
    [timeseriesQuery.data?.items],
  );

  const transactions = useMemo(() => transactionsQuery.data?.transactions ?? [], [transactionsQuery.data?.transactions]);

  return (
    <div className="space-y-4">
      <div className="grid gap-4 md:grid-cols-3">
        <KPICard
          label="Total Distributed"
          value={overviewQuery.data?.total_distributed ?? 0}
          icon={Save}
          description="All-time credit distribution"
        />
        <KPICard
          label="Total Consumed"
          value={overviewQuery.data?.total_consumed ?? 0}
          icon={Save}
          description="All-time credit usage"
        />
        <KPICard
          label="Top Consumers"
          value={overviewQuery.data?.top_consumers?.length ?? 0}
          icon={Save}
          description="Users with highest usage"
        />
      </div>

      <BarChart data={chartData} title="Credit Consumption (24h)" />

      <Card>
        <CardHeader>
          <CardTitle>Transactions</CardTitle>
          <CardDescription>Recent credit transactions for selected user.</CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex flex-wrap gap-2">
            {(overviewQuery.data?.top_consumers ?? []).map((consumer) => (
              <Button
                key={consumer.user_id}
                variant={selectedUserID === consumer.user_id ? "default" : "outline"}
                onClick={() => setSelectedUserID(consumer.user_id)}
              >
                {consumer.name}
              </Button>
            ))}
          </div>
          <DataTable<CreditTransaction, unknown>
            data={transactions}
            columns={TRANSACTION_COLUMNS}
            searchColumn="type"
            searchPlaceholder="Filter type..."
            fileName="credit-transactions"
          />
        </CardContent>
      </Card>

      <div className="grid gap-4 xl:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Pricing Editor</CardTitle>
            <CardDescription>Method multipliers as JSON object.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <Label htmlFor="method-multipliers">Method Multipliers</Label>
            <Textarea
              id="method-multipliers"
              className="min-h-44 font-mono text-xs"
              value={methodMultipliers}
              onChange={(event) => setMethodMultipliers(event.target.value)}
            />
            <Button onClick={() => saveBillingMutation.mutate()} disabled={saveBillingMutation.isPending}>
              Save Pricing
            </Button>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Route Costs</CardTitle>
            <CardDescription>Per-route cost overrides.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <Label htmlFor="route-costs">Route Costs JSON</Label>
            <Textarea
              id="route-costs"
              className="min-h-44 font-mono text-xs"
              value={routeCosts}
              onChange={(event) => setRouteCosts(event.target.value)}
            />
            <Button onClick={() => saveRouteCostsMutation.mutate()} disabled={saveRouteCostsMutation.isPending}>
              Save Route Costs
            </Button>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
