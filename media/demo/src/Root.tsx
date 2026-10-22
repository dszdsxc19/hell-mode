import { Composition } from "remotion";
import { HellMode } from "./Composition";

export const RemotionRoot: React.FC = () => {
  return (
    <Composition
      id="HellMode"
      component={HellMode}
      durationInFrames={450}
      fps={30}
      width={1200}
      height={760}
    />
  );
};
