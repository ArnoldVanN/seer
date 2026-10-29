"use client"

import { useState } from "react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import type { ResultView } from "@/lib/types"

const PAGE = 10

export function RecentChecks({ results }: { results: ResultView[] }) {
  const [filter, setFilter] = useState<"all" | "failed">("all")
  const [shown, setShown] = useState(PAGE)
  const failed = results.filter((r) => !r.success).length
  const rows = filter === "failed" ? results.filter((r) => !r.success) : results

  return (
    <section className="seer-results">
      <div className="mb-2 flex items-center justify-between gap-3">
        <h2 className="text-sm font-semibold">Recent checks</h2>
        <ToggleGroup
          type="single"
          size="sm"
          variant="outline"
          value={filter}
          onValueChange={(v) => {
            if (v) {
              setFilter(v as "all" | "failed")
              setShown(PAGE)
            }
          }}
        >
          <ToggleGroupItem value="all">All</ToggleGroupItem>
          <ToggleGroupItem value="failed">Failures{failed > 0 && ` (${failed})`}</ToggleGroupItem>
        </ToggleGroup>
      </div>
      <div className="rounded-xl border bg-card">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Time (UTC)</TableHead>
              <TableHead>Result</TableHead>
              <TableHead className="text-right">Response</TableHead>
              <TableHead className="w-1/2">Error</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.slice(0, shown).map((r) => (
              <TableRow key={r.checkedAt} data-success={r.success}>
                <TableCell className="tabular-nums">{r.checkedAt.replace("T", " ").slice(0, 19)}</TableCell>
                <TableCell>
                  {r.success ? (
                    <Badge variant="secondary">OK</Badge>
                  ) : (
                    <Badge variant="destructive">Failed</Badge>
                  )}
                </TableCell>
                <TableCell className="text-right tabular-nums">{r.responseTimeMs} ms</TableCell>
                <TableCell className="seer-error max-w-0 truncate font-mono text-xs text-muted-foreground" title={r.error}>
                  {r.error}
                </TableCell>
              </TableRow>
            ))}
            {rows.length === 0 && (
              <TableRow>
                <TableCell colSpan={4} className="py-6 text-center text-muted-foreground">
                  {filter === "failed" ? "No failures in the recent checks." : "No checks yet."}
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>
      {rows.length > shown && (
        <div className="mt-2 flex justify-center">
          <Button variant="ghost" size="sm" onClick={() => setShown((n) => n + PAGE * 2)}>
            Show more ({rows.length - shown} more)
          </Button>
        </div>
      )}
    </section>
  )
}
