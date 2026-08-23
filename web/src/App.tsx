import { Route, Routes } from "react-router-dom";

import DefaultLayout from "./layouts/default";
// 页面组件
import LoginPage from "./pages/login";
import DashboardPage from "./pages/dashboard";
import TunnelsPage from "./pages/tunnels";
import TunnelDetailsPage from "./pages/tunnels/details";
import SubscriptionsPage from "./pages/subscriptions";
import EndpointsPage from "./pages/endpoints";
import SettingsPage from "./pages/settings";
import VersionHistoryPage from "./pages/settings/version-history";
import SetupGuidePage from "./pages/setup-guide";
import SetupPage from "./pages/setup";
import OAuthErrorPage from "./pages/oauth-error";
import OAuthSuccessPage from "./pages/oauth-success";
import DebugPage from "./pages/debug";
import EndpointDetailsPage from "./pages/endpoints/details";
import EndpointSSEDebugPage from "./pages/endpoints/sse-debug";
import ExamplesPage from "./pages/examples";
import IconComparisonPage from "./pages/icon-comparison";

function App() {
  return (
    <DefaultLayout>
      <Routes>
        <Route element={<LoginPage />} path="/login" />
        <Route element={<OAuthErrorPage />} path="/oauth-error" />
        <Route element={<OAuthSuccessPage />} path="/oauth-success" />
        <Route element={<SetupGuidePage />} path="/setup-guide" />
        <Route element={<SetupPage />} path="/setup" />
        <Route element={<DashboardPage />} path="/dashboard" />
        <Route element={<TunnelsPage />} path="/tunnels" />
        <Route element={<TunnelDetailsPage />} path="/tunnels/details" />
        <Route element={<SubscriptionsPage />} path="/subscriptions" />
        <Route element={<EndpointsPage />} path="/endpoints" />
        <Route element={<EndpointDetailsPage />} path="/endpoints/details" />
        <Route element={<EndpointSSEDebugPage />} path="/endpoints/sse-debug" />
        <Route element={<SettingsPage />} path="/settings" />
        <Route
          element={<VersionHistoryPage />}
          path="/settings/version-history"
        />
        <Route element={<ExamplesPage />} path="/docs" />
        <Route element={<DebugPage />} path="/debug" />
        <Route element={<IconComparisonPage />} path="/icon-comparison" />
        <Route element={<DashboardPage />} path="/" />
      </Routes>
    </DefaultLayout>
  );
}

export default App;
