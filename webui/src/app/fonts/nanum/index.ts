import localFont from "next/font/local";

const nanumGothic = localFont({
    src: [
        {
            path: "./NanumGothic.woff2",
            weight: "400",
            style: "normal",
        },
        {
            path: "./NanumGothicLight.woff2",
            weight: "300",
            style: "normal",
        },
        {
            path: "./NanumGothicBold.woff2",
            weight: "700",
            style: "normal",
        },
        {
            path: "./NanumGothicExtraBold.woff2",
            weight: "800",
            style: "normal",
        },
    ],
    variable: "--font-nanum-gothic",
    display: "swap",
});

export default nanumGothic;
