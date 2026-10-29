// Kept free of server-only so proxy.ts can use it too.

// In-cluster URL of the Go API's public port.
export const publicApiUrl = () => process.env.SEER_PUBLIC_API_URL || "http://localhost:8080"
