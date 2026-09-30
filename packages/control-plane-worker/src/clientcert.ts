export const CLIENT_CERT_HEADER = "X-K8flare-Client-Cert";

interface TLSClientAuth {
  certPresented?: string;
  certVerified?: string;
  certRFC9440?: string;
  certRFC9440TooLarge?: boolean;
}

export function verifiedClientCert(cf: unknown): string | null {
  const auth = (cf as { tlsClientAuth?: TLSClientAuth | null } | undefined)?.tlsClientAuth;
  if (!auth || auth.certPresented !== "1" || auth.certVerified !== "SUCCESS") return null;
  if (auth.certRFC9440TooLarge || !auth.certRFC9440) return null;
  return auth.certRFC9440;
}

export function withClientCert(request: Request, cf: unknown = request.cf): Request {
  const headers = new Headers(request.headers);
  headers.delete(CLIENT_CERT_HEADER);
  const cert = verifiedClientCert(cf);
  if (cert) headers.set(CLIENT_CERT_HEADER, cert);
  return new Request(request, { headers });
}
