export { handleResourceCRUD } from "./crud.ts";
export { applyCommonDefaults } from "./defaults.ts";
export {
  buildDiscoveryResources,
  buildDiscoveryGroup,
  injectCustomAPIGroup,
  injectOpenAPIV3Path,
} from "./discovery.ts";
export { parseCustomGroupPath } from "./path-parser.ts";
export { dwStub, crKey, crListPrefix, crGet, crList, crPut, crDelete } from "./storage.ts";
export { buildGroupOpenAPIDocument } from "./openapi.ts";
export type { OpenAPIKindSpec } from "./openapi.ts";
export type {
  ObjectMeta,
  CRDConfig,
  CustomResource,
  APIResourceDescriptor,
  ParsedPath,
  CRGetResult,
} from "./types.ts";
