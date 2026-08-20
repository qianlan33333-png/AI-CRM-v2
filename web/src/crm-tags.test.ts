import { describe, expect, it } from "vitest";
import { parseCRMTagCatalog } from "./crm-tags";

describe("CRM local tag catalog decoder", () => {
  it("accepts only exact local groups and tags", () => {
    expect(parseCRMTagCatalog({ groups: [{ id: 1, name: "意向", sort_order: 0 }], tags: [{ id: 2, group_id: 1, group_name: "意向", name: "高", sort_order: 0 }] })).toMatchObject({ groups: [{ id: 1 }], tags: [{ id: 2 }] });
  });
  it("rejects cross-group names, duplicate IDs, and expanded payloads", () => {
    expect(parseCRMTagCatalog({ groups: [{ id: 1, name: "意向", sort_order: 0 }], tags: [{ id: 2, group_id: 1, group_name: "错误", name: "高", sort_order: 0 }] })).toBeUndefined();
    expect(parseCRMTagCatalog({ groups: [{ id: 1, name: "意向", sort_order: 0 }], tags: [], extra: true })).toBeUndefined();
  });
});
