import { LogTail } from "@/components/logs/LogTail";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Terminal, FileText, AlertCircle } from "lucide-react";

export function SystemLogsPage() {
  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold tracking-tight">System Logs</h1>
          <p className="text-muted-foreground">
            Real-time log streaming and analysis
          </p>
        </div>
        <Badge variant="outline">Live</Badge>
      </div>

      <Tabs defaultValue="live" className="space-y-4">
        <TabsList>
          <TabsTrigger value="live">
            <Terminal className="h-4 w-4 mr-1" />
            Live Tail
          </TabsTrigger>
          <TabsTrigger value="errors">
            <AlertCircle className="h-4 w-4 mr-1" />
            Errors
          </TabsTrigger>
          <TabsTrigger value="audit">
            <FileText className="h-4 w-4 mr-1" />
            Audit
          </TabsTrigger>
        </TabsList>

        <TabsContent value="live">
          <LogTail
            maxLogs={1000}
            showFilters={true}
            showExport={true}
            height="h-[600px]"
            title="Live System Logs"
          />
        </TabsContent>

        <TabsContent value="errors">
          <LogTail
            maxLogs={500}
            showFilters={true}
            showExport={true}
            height="h-[600px]"
            title="Error Logs"
          />
        </TabsContent>

        <TabsContent value="audit">
          <Card>
            <CardHeader>
              <CardTitle>Audit Log Stream</CardTitle>
              <CardDescription>
                Real-time audit events from the audit log system
              </CardDescription>
            </CardHeader>
            <CardContent>
              <LogTail
                maxLogs={500}
                showFilters={false}
                showExport={true}
                height="h-[500px]"
                title="Audit Events"
              />
            </CardContent>
          </Card>
        </TabsContent>
      </Tabs>
    </div>
  );
}
