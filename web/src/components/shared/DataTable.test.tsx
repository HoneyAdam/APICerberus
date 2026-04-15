import { describe, expect, it, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { DataTable } from "./DataTable";
import type { ColumnDef } from "@tanstack/react-table";

interface TestRow {
  id: string;
  name: string;
  email: string;
  status: string;
}

const testData: TestRow[] = [
  { id: "1", name: "Alice", email: "alice@example.com", status: "active" },
  { id: "2", name: "Bob", email: "bob@example.com", status: "suspended" },
  { id: "3", name: "Charlie", email: "charlie@example.com", status: "active" },
];

function sortableColumns(): ColumnDef<TestRow>[] {
  return [
    { accessorKey: "name", header: "Name" },
    { accessorKey: "email", header: "Email" },
    { accessorKey: "status", header: "Status" },
  ];
}

describe("DataTable Accessibility", () => {
  it("renders a grid role on the table", () => {
    render(<DataTable columns={sortableColumns()} data={testData} />);
    const grid = screen.getByRole("grid");
    expect(grid).toBeInTheDocument();
  });

  it("renders a region with aria-label", () => {
    render(<DataTable columns={sortableColumns()} data={testData} />);
    const region = screen.getByRole("region", { name: "Data table" });
    expect(region).toBeInTheDocument();
  });

  it("renders columnheader role on all header cells", () => {
    render(<DataTable columns={sortableColumns()} data={testData} />);
    const headers = screen.getAllByRole("columnheader");
    expect(headers).toHaveLength(3);
    expect(headers.map((h) => h.textContent)).toEqual(["Name", "Email", "Status"]);
  });

  it("renders aria-sort=none on unsorted columns", () => {
    render(<DataTable columns={sortableColumns()} data={testData} />);
    const headers = screen.getAllByRole("columnheader");
    for (const header of headers) {
      expect(header).toHaveAttribute("aria-sort", "none");
    }
  });

  it("sets aria-sort=ascending when column is sorted ascending", async () => {
    const user = userEvent.setup();
    render(<DataTable columns={sortableColumns()} data={testData} />);

    const nameButton = screen.getByLabelText(/Sort by name, not sorted/);
    await user.click(nameButton);

    const headers = screen.getAllByRole("columnheader");
    const nameHeader = headers.find((h) => h.textContent?.includes("Name"));
    expect(nameHeader).toHaveAttribute("aria-sort", "ascending");
  });

  it("sets aria-sort=descending when column is sorted descending", async () => {
    const user = userEvent.setup();
    render(<DataTable columns={sortableColumns()} data={testData} />);

    const nameButton = screen.getByLabelText(/Sort by name, not sorted/);
    await user.click(nameButton); // asc
    const ascButton = screen.getByLabelText(/Sort by name, sorted ascending/);
    await user.click(ascButton); // desc

    const headers = screen.getAllByRole("columnheader");
    const nameHeader = headers.find((h) => h.textContent?.includes("Name"));
    expect(nameHeader).toHaveAttribute("aria-sort", "descending");
  });

  it("cycles aria-sort through none -> ascending -> descending -> none", async () => {
    const user = userEvent.setup();
    render(<DataTable columns={sortableColumns()} data={testData} />);

    const headers = () => screen.getAllByRole("columnheader");
    const nameHeader = () => headers().find((h) => h.textContent?.includes("Name"))!;

    // Initial: none
    expect(nameHeader()).toHaveAttribute("aria-sort", "none");

    // Click 1: ascending
    await user.click(screen.getByLabelText(/Sort by name, not sorted/));
    expect(nameHeader()).toHaveAttribute("aria-sort", "ascending");

    // Click 2: descending
    await user.click(screen.getByLabelText(/Sort by name, sorted ascending/));
    expect(nameHeader()).toHaveAttribute("aria-sort", "descending");

    // Click 3: back to none
    await user.click(screen.getByLabelText(/Sort by name, sorted descending/));
    expect(nameHeader()).toHaveAttribute("aria-sort", "none");
  });

  it("renders gridcell role on data cells", () => {
    render(<DataTable columns={sortableColumns()} data={testData} />);
    const cells = screen.getAllByRole("gridcell");
    // 3 rows × 3 columns = 9 cells
    expect(cells).toHaveLength(9);
  });

  it("renders row role on data rows", () => {
    render(<DataTable columns={sortableColumns()} data={testData} />);
    // header row + 3 data rows = 4 rows total
    const rows = screen.getAllByRole("row");
    expect(rows).toHaveLength(4);
  });

  it("sort button has descriptive aria-label", () => {
    render(<DataTable columns={sortableColumns()} data={testData} />);
    const nameButton = screen.getByLabelText(/Sort by name, not sorted, activate to sort ascending/);
    expect(nameButton).toBeInTheDocument();
  });

  it("updates aria-label when sort direction changes", async () => {
    const user = userEvent.setup();
    render(<DataTable columns={sortableColumns()} data={testData} />);

    // Initial
    expect(screen.getByLabelText(/Sort by name, not sorted/)).toBeInTheDocument();

    // Click to sort ascending
    await user.click(screen.getByLabelText(/Sort by name, not sorted/));
    expect(screen.getByLabelText(/Sort by name, sorted ascending, activate to sort descending/)).toBeInTheDocument();

    // Click to sort descending
    await user.click(screen.getByLabelText(/Sort by name, sorted ascending/));
    expect(screen.getByLabelText(/Sort by name, sorted descending, activate to remove sort/)).toBeInTheDocument();
  });

  it("renders empty message when no data", () => {
    render(<DataTable columns={sortableColumns()} data={[]} emptyMessage="No users found." />);
    expect(screen.getByText("No users found.")).toBeInTheDocument();
  });

  it("sort button is focusable via keyboard", async () => {
    const user = userEvent.setup();
    render(<DataTable columns={sortableColumns()} data={testData} />);

    const nameButton = screen.getByLabelText(/Sort by name, not sorted/);
    nameButton.focus();
    expect(nameButton).toHaveFocus();

    await user.keyboard("{Enter}");
    expect(screen.getByLabelText(/Sort by name, sorted ascending/)).toBeInTheDocument();
  });

  it("maintains correct sort state independently per column", async () => {
    const user = userEvent.setup();
    render(<DataTable columns={sortableColumns()} data={testData} />);

    // Sort by Name ascending
    await user.click(screen.getByLabelText(/Sort by name, not sorted/));

    const headers = screen.getAllByRole("columnheader");
    const nameHeader = headers.find((h) => h.textContent?.includes("Name"))!;
    const emailHeader = headers.find((h) => h.textContent?.includes("Email"))!;

    expect(nameHeader).toHaveAttribute("aria-sort", "ascending");
    expect(emailHeader).toHaveAttribute("aria-sort", "none");
  });
});
