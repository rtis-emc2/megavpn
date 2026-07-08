import { NavLink } from 'react-router-dom';

export type PageTab = {
  label: string;
  to: string;
};

export function PageTabs({ tabs }: { tabs: PageTab[] }) {
  return (
    <nav className="tabs">
      {tabs.map((tab) => (
        <NavLink className="tab-link" key={tab.to} to={tab.to} end>
          {tab.label}
        </NavLink>
      ))}
    </nav>
  );
}

export const SectionTabs = PageTabs;
