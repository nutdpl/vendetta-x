/* Vendetta/X toolchain verification: 16-bit real-mode DOS, large model. */
#include <stdio.h>

int main(void)
{
    printf("Vendetta/X toolchain lives.\r\n");
    printf("CP437 check: \xB0\xB1\xB2\xDB \x03 \xDB\xB2\xB1\xB0\r\n");
    return 0;
}
