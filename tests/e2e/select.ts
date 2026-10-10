import { expect, type Page } from "@playwright/test";

export async function selectOption(page: Page, label: string, option: string) {
  await page.getByRole("combobox", { name: label, exact: true }).click();
  await page
    .getByRole("combobox", {
      name: `Search ${label.toLowerCase()}`,
      exact: true,
    })
    .fill(option);
  await page.getByRole("option", { name: option, exact: true }).click();
  await expect(
    page.getByRole("combobox", { name: label, exact: true }),
  ).toHaveAttribute("aria-expanded", "false");
}
