import { useMemo, useState } from "react";
import type { ColumnDef } from "@tanstack/react-table";
import { toast } from "sonner";
import { DataTable } from "@/components/shared/DataTable";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  usePortalCreditTransactions,
  usePortalCreditsBalance,
  usePortalForecast,
  usePortalPurchaseCredits,
} from "@/hooks/use-portal";
import type { PortalCreditTransaction } from "@/lib/portal-types";

const TRANSACTION_COLUMNS: ColumnDef<PortalCreditTransaction>[] = [
  {
    accessorKey: "created_at",
    header: "Date",
  },
  {
    accessorKey: "type",
    header: "Type",
  },
  {
    accessorKey: "amount",
    header: "Amount",
  },
  {
    accessorKey: "balance_after",
    header: "Balance After",
  },
  {
    accessorKey: "description",
    header: "Description",
  },
];

export function PortalCreditsPage() {
  const [amount, setAmount] = useState("100");
  const [description, setDescription] = useState("Self top-up");

  const balanceQuery = usePortalCreditsBalance();
  const transactionsQuery = usePortalCreditTransactions({ limit: 200, offset: 0 });
  const forecastQuery = usePortalForecast();
  const purchaseMutation = usePortalPurchaseCredits();

  const transactions = useMemo(
    () => transactionsQuery.data?.transactions ?? [],
    [transactionsQuery.data?.transactions],
  );

  const onPurchase = async () => {
    const parsedAmount = Number(amount);
    if (!Number.isFinite(parsedAmount) || parsedAmount <= 0) {
      toast.error("Amount must be a positive number");
      return;
    }
    try {
      await purchaseMutation.mutateAsync({
        amount: parsedAmount,
        description,
      });
      toast.success("Credits purchased");
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to purchase credits");
    }
  };

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-2xl font-semibold">Credits</h2>
        <p className="text-sm text-muted-foreground">Track balance, purchases and consumption forecast.</p>
      </div>

      <div className="grid gap-3 md:grid-cols-3">
        <Card>
          <CardHeader>
            <CardTitle>Current Balance</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-4xl font-semibold">{balanceQuery.data?.balance ?? 0}</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Avg Daily Consumption</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-2xl font-semibold">{(forecastQuery.data?.average_daily_consumption ?? 0).toFixed(2)}</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Projected Days Remaining</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-2xl font-semibold">{(forecastQuery.data?.projected_days_remaining ?? 0).toFixed(1)}</p>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Purchase Credits</CardTitle>
          <CardDescription>Top up your own balance instantly.</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3 md:grid-cols-[180px_1fr_auto]">
          <Input value={amount} onChange={(event) => setAmount(event.target.value)} placeholder="Amount" />
          <Input
            value={description}
            onChange={(event) => setDescription(event.target.value)}
            placeholder="Purchase reason"
          />
          <Button onClick={onPurchase} disabled={purchaseMutation.isPending}>
            Purchase
          </Button>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Transactions</CardTitle>
        </CardHeader>
        <CardContent>
          <DataTable<PortalCreditTransaction, unknown>
            data={transactions}
            columns={TRANSACTION_COLUMNS}
            searchColumn="description"
            searchPlaceholder="Search description..."
            initialPageSize={10}
          />
        </CardContent>
      </Card>
    </div>
  );
}
