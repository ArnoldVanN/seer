import Link from "next/link"
import { EmptyState } from "@/components/DashboardPage"

export default function NotFound() {
  return (
    <EmptyState title="Not found">
      This page doesn&apos;t exist or you don&apos;t have access to it.{" "}
      <Link href="/" className="text-primary underline underline-offset-2">
        Go to the status page
      </Link>
    </EmptyState>
  )
}
