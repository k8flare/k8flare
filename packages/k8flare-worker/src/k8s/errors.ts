/**
 * Return a Response containing a Kubernetes Status error JSON body.
 */
export function dwError(status: number, message: string): Response {
  return Response.json(
    { kind: "Status", apiVersion: "v1", status: "Failure", message, code: status },
    { status },
  );
}
