import { useEffect, useMemo, useState } from 'react';
import type { ReactElement } from 'react';
import {
  Avatar,
  Button,
  Dropdown,
  Layout,
  Menu,
  Space,
  Tag,
  Typography,
} from 'antd';
import type { MenuProps } from 'antd';
import {
  BugOutlined,
  DashboardOutlined,
  FileSearchOutlined,
  LogoutOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined,
  ToolOutlined,
  UserOutlined,
} from '@ant-design/icons';
import {
  BrowserRouter,
  Navigate,
  Outlet,
  Route,
  Routes,
  useLocation,
  useNavigate,
} from 'react-router-dom';
import { TOKEN_KEY } from './api/client';
import { useAuth } from './hooks/useAuth';
import { buildMenuItems, getMenuFromCache } from './utils/menu';
import type { MenuGroup } from './utils/menu';
import CampaignAnalysis from './pages/CampaignAnalysis';
import Dashboard from './pages/Dashboard';
import Login from './pages/Login';
import SystemDebug from './pages/SystemDebug';
import TaskManagement from './pages/TaskManagement';

const { Sider, Header, Content } = Layout;

/** 路由守卫：未登录（无 token）时跳转登录页 */
function RequireAuth({ children }: { children: ReactElement }) {
  const location = useLocation();
  if (!localStorage.getItem(TOKEN_KEY)) {
    return <Navigate to="/login" replace state={{ from: location }} />;
  }
  return children;
}

/** 主布局：Sider 动态菜单 + Header（用户信息/登出）+ Content 路由区 */
function MainLayout() {
  const { user, isAdmin, logout } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const [collapsed, setCollapsed] = useState(false);
  const [menuGroups, setMenuGroups] = useState<MenuGroup[]>([]);

  useEffect(() => {
    // 先读缓存快速渲染，再异步刷新
    const cached = getMenuFromCache();
    if (cached) setMenuGroups(cached);
    buildMenuItems()
      .then(setMenuGroups)
      .catch(() => {
        /* 拉取失败时保留缓存内容，静默处理 */
      });
  }, []);

  const menuItems = useMemo<NonNullable<MenuProps['items']>>(() => {
    const items: NonNullable<MenuProps['items']> = [
      { key: '/', icon: <DashboardOutlined />, label: '仪表盘' },
      { key: '/tasks', icon: <FileSearchOutlined />, label: '任务管理' },
      { key: '/debug', icon: <ToolOutlined />, label: '系统调试' },
    ];
    // 动态分组：专项扫描 / 普通扫描（来自任务类型）
    for (const group of menuGroups) {
      items.push({
        key: `group:${group.group}`,
        type: 'group',
        label: group.group,
        children: group.items.map((item) => ({
          key: `/campaign/${encodeURIComponent(item.name)}`,
          icon: <BugOutlined />,
          label: item.display_name,
        })),
      });
    }
    return items;
  }, [menuGroups]);

  const selectedKey = location.pathname;

  const userMenuItems: MenuProps['items'] = [
    { key: 'role', label: `角色：${user?.role ?? '-'}`, disabled: true },
    { type: 'divider' },
    { key: 'logout', label: '退出登录', icon: <LogoutOutlined /> },
  ];

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Sider
        collapsible
        collapsed={collapsed}
        onCollapse={setCollapsed}
        theme="dark"
        width={220}
      >
        <div className="app-logo">{collapsed ? 'CS' : 'Code-Shield'}</div>
        <Menu
          theme="dark"
          mode="inline"
          items={menuItems}
          selectedKeys={[selectedKey]}
          onClick={({ key }) => navigate(key)}
        />
      </Sider>
      <Layout>
        <Header
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
            paddingInline: 16,
          }}
        >
          <Button
            type="text"
            icon={collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
            onClick={() => setCollapsed(!collapsed)}
          />
          <Space size={16}>
            {isAdmin && <Tag color="gold">管理员</Tag>}
            <Dropdown
              menu={{
                items: userMenuItems,
                onClick: ({ key }) => {
                  if (key === 'logout') logout();
                },
              }}
            >
              <Space style={{ cursor: 'pointer' }}>
                <Avatar size="small" icon={<UserOutlined />} />
                <Typography.Text>{user?.username ?? '-'}</Typography.Text>
              </Space>
            </Dropdown>
          </Space>
        </Header>
        <Content style={{ padding: 16 }}>
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  );
}

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route
          element={
            <RequireAuth>
              <MainLayout />
            </RequireAuth>
          }
        >
          <Route path="/" element={<Dashboard />} />
          <Route path="/tasks" element={<TaskManagement />} />
          <Route path="/debug" element={<SystemDebug />} />
          <Route path="/campaign/:name" element={<CampaignAnalysis />} />
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </BrowserRouter>
  );
}