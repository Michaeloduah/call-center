import { Alert } from "@/components/ui/Alert";
import { noticeText } from "@/lib/notices";

/** Shows a whitelisted success message passed through ?notice= after a redirect. */
export function Notice({ value }: { value: string | string[] | undefined }) {
  const text = noticeText(Array.isArray(value) ? value[0] : value);
  return text ? (
    <div className="mb-6">
      <Alert tone={text.includes("could not") ? "warning" : "success"}>{text}</Alert>
    </div>
  ) : null;
}
