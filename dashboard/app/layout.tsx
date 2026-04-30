import "./globals.css";
import Nav from "../components/nav";
import { getViewerOptional } from "../lib/api";

export default async function RootLayout({ children }: { children: React.ReactNode }) {
  const viewer = await getViewerOptional();
  return (
    <html lang="en">
      <body>
        <div className="page-shell">
          <Nav viewer={viewer} />
          <main className="page-main">{children}</main>
        </div>
      </body>
    </html>
  );
}
